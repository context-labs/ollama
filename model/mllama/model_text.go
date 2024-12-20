package mllama

import (
	"math"
	"slices"

	"github.com/ollama/ollama/ml"
	"github.com/ollama/ollama/ml/nn"
	"github.com/ollama/ollama/model"
)

type TextSelfAttention struct {
	Query  *nn.Linear `ggml:"attn_q"`
	Key    *nn.Linear `ggml:"attn_k"`
	Value  *nn.Linear `ggml:"attn_v"`
	Output *nn.Linear `ggml:"attn_output"`
}

func (sa *TextSelfAttention) Forward(ctx ml.Context, hiddenState, positions, mask ml.Tensor, cache model.Cache, opts *TextModelOptions) ml.Tensor {
	batchSize := hiddenState.Dim(1)
	headDim := opts.hiddenSize / opts.numHeads

	query := sa.Query.Forward(ctx, hiddenState)
	query = query.Reshape(ctx, headDim, opts.numHeads, batchSize)
	query = query.Rope(ctx, positions, opts.RopeFactors, opts.ropeDim, opts.ropeBase, opts.ropeScale)

	key := sa.Key.Forward(ctx, hiddenState)
	key = key.Reshape(ctx, headDim, opts.numKVHeads, batchSize)
	key = key.Rope(ctx, positions, opts.RopeFactors, opts.ropeDim, opts.ropeBase, opts.ropeScale)

	value := sa.Value.Forward(ctx, hiddenState)
	value = value.Reshape(ctx, headDim, opts.numKVHeads, batchSize)

	key, value = cache.Put(ctx, key, value, cache.Options)

	query = query.Permute(ctx, 0, 2, 1, 3).Contiguous(ctx)
	key = key.Permute(ctx, 0, 2, 1, 3).Contiguous(ctx)
	value = value.Permute(ctx, 1, 2, 0, 3).Contiguous(ctx)

	scores := key.Mulmat(ctx, query)
	scores = scores.Scale(ctx, 1.0/math.Sqrt(float64(headDim)))

	if mask != nil {
		scores = scores.Add(ctx, mask)
	}

	scores = scores.Softmax(ctx)

	attention := value.Mulmat(ctx, scores)
	attention = attention.Permute(ctx, 0, 2, 1, 3).Contiguous(ctx)
	attention = attention.Reshape(ctx, opts.hiddenSize, batchSize)

	return sa.Output.Forward(ctx, attention)
}

type TextMLP struct {
	Up   *nn.Linear `ggml:"ffn_up"`
	Down *nn.Linear `ggml:"ffn_down"`
	Gate *nn.Linear `ggml:"ffn_gate"`
}

func (mlp *TextMLP) Forward(ctx ml.Context, hiddenState ml.Tensor, opts *TextModelOptions) ml.Tensor {
	hiddenState = mlp.Gate.Forward(ctx, hiddenState).SILU(ctx).Mul(ctx, mlp.Up.Forward(ctx, hiddenState))
	return mlp.Down.Forward(ctx, hiddenState)
}

type TextSelfAttentionDecoderLayer struct {
	AttentionNorm *nn.RMSNorm `ggml:"attn_norm"`
	SelfAttention *TextSelfAttention

	MLPNorm *nn.RMSNorm `ggml:"ffn_norm"`
	MLP     *TextMLP
}

func (d *TextSelfAttentionDecoderLayer) Forward(ctx ml.Context, hiddenState, positions, mask, _, _ ml.Tensor, cache model.Cache, opts *TextModelOptions) ml.Tensor {
	residual := hiddenState

	hiddenState = d.AttentionNorm.Forward(ctx, hiddenState, opts.eps)
	hiddenState = d.SelfAttention.Forward(ctx, hiddenState, positions, mask, cache, opts)
	hiddenState = hiddenState.Add(ctx, residual)
	residual = hiddenState

	hiddenState = d.MLPNorm.Forward(ctx, hiddenState, opts.eps)
	hiddenState = d.MLP.Forward(ctx, hiddenState, opts)
	return hiddenState.Add(ctx, residual)
}

type TextCrossAttention struct {
	QueryNorm *nn.RMSNorm `ggml:"cross_attn_q_norm"`
	Query     *nn.Linear  `ggml:"cross_attn_q_proj"`
	KeyNorm   *nn.RMSNorm `ggml:"cross_attn_k_norm"`
	Key       *nn.Linear  `ggml:"cross_attn_k_proj"`
	Value     *nn.Linear  `ggml:"cross_attn_v_proj"`
	Output    *nn.Linear  `ggml:"cross_attn_o_proj"`
}

func (ca *TextCrossAttention) Forward(ctx ml.Context, hiddenState, crossAttentionStates ml.Tensor, cache model.Cache, opts *TextModelOptions) ml.Tensor {
	batchSize := hiddenState.Dim(1)
	headDim := opts.hiddenSize / opts.numHeads
	numVisionTokens, numTiles := crossAttentionStates.Dim(1), crossAttentionStates.Dim(2)

	query := ca.Query.Forward(ctx, hiddenState)
	query = query.Reshape(ctx, headDim, opts.numHeads, batchSize)
	query = ca.QueryNorm.Forward(ctx, query, opts.eps)

	key := ca.Key.Forward(ctx, crossAttentionStates)
	key = key.Reshape(ctx, headDim, opts.numKVHeads, numVisionTokens*numTiles)
	key = ca.KeyNorm.Forward(ctx, key, opts.eps)

	value := ca.Value.Forward(ctx, crossAttentionStates)
	value = value.Reshape(ctx, headDim, opts.numKVHeads, numVisionTokens*numTiles)

	// TODO cache key, value

	query = query.Permute(ctx, 0, 2, 1, 3).Contiguous(ctx)
	key = key.Permute(ctx, 0, 2, 1, 3).Contiguous(ctx)
	value = value.Permute(ctx, 1, 2, 0, 3).Contiguous(ctx)

	scores := key.Mulmat(ctx, query)
	scores = scores.Scale(ctx, 1.0/math.Sqrt(float64(headDim)))
	scores = scores.Softmax(ctx)

	attention := value.Mulmat(ctx, scores)
	attention = attention.Permute(ctx, 0, 2, 1, 3).Contiguous(ctx)
	attention = attention.Reshape(ctx, opts.hiddenSize, batchSize)

	return ca.Output.Forward(ctx, attention)
}

type TextCrossAttentionDecoderLayer struct {
	AttentionNorm  *nn.RMSNorm `ggml:"attn_norm"`
	CrossAttention *TextCrossAttention
	AttentionGate  ml.Tensor `ggml:"cross_attn_attn_gate"`

	MLPNorm *nn.RMSNorm `ggml:"ffn_norm"`
	MLP     *TextMLP
	MLPGate ml.Tensor `ggml:"cross_attn_mlp_gate"`
}

func (d TextCrossAttentionDecoderLayer) Forward(ctx ml.Context, hiddenState, _, _, crossAttentionStates, crossAttentionMask ml.Tensor, cache model.Cache, opts *TextModelOptions) ml.Tensor {
	residual := hiddenState

	hiddenState = d.AttentionNorm.Forward(ctx, hiddenState, opts.eps)
	hiddenState = d.CrossAttention.Forward(ctx, hiddenState, crossAttentionStates, cache, opts)
	hiddenState = hiddenState.Mul(ctx, d.AttentionGate.Tanh(ctx))
	hiddenState = hiddenState.Add(ctx, residual)
	residual = hiddenState

	hiddenState = d.MLPNorm.Forward(ctx, hiddenState, opts.eps)
	hiddenState = d.MLP.Forward(ctx, hiddenState, opts)
	hiddenState = hiddenState.Mul(ctx, d.MLPGate.Tanh(ctx))
	return hiddenState.Add(ctx, residual)
}

type TextDecoderLayer interface {
	Forward(ctx ml.Context, hiddenState, positionIDs, mask, crossAttentionStates, crossAttentionMask ml.Tensor, cache model.Cache, opts *TextModelOptions) ml.Tensor
}

type TextDecoder struct {
	Layers []TextDecoderLayer
}

func (d *TextDecoder) Forward(ctx ml.Context, hiddenState, positionIDs, mask, crossAttentionStates, crossAttentionMask ml.Tensor, cache model.Cache, opts *TextModelOptions) ml.Tensor {
	for i, layer := range d.Layers {
		if !slices.Contains(opts.crossAttentionLayers, uint32(i)) || crossAttentionStates != nil {
			hiddenState = layer.Forward(ctx, hiddenState, positionIDs, mask, crossAttentionStates, crossAttentionMask, cache.Sub(i), opts)
		}
	}

	return hiddenState
}

type TextModelOptions struct {
	RopeFactors ml.Tensor `ggml:"rope_freqs.weight"`

	hiddenSize, numHeads, numKVHeads int64
	eps, ropeBase, ropeScale         float32
	ropeDim                          uint32

	crossAttentionLayers []uint32
}

type TextModel struct {
	TokenEmbedding *nn.Embedding `ggml:"token_embd"`
	Transformer    *TextDecoder  `ggml:"blk"`
	OutputNorm     *nn.RMSNorm   `ggml:"output_norm"`
	Output         *nn.Linear    `ggml:"output"`

	*TextModelOptions
}

func (m *TextModel) Forward(ctx ml.Context, inputIDs, positionIDs, mask, crossAttentionStates, crossAttentionMask ml.Tensor, cache model.Cache) ml.Tensor {
	hiddenState := m.TokenEmbedding.Forward(ctx, inputIDs)
	hiddenState = m.Transformer.Forward(ctx, hiddenState, positionIDs, mask, crossAttentionStates, crossAttentionMask, cache, m.TextModelOptions)
	hiddenState = m.OutputNorm.Forward(ctx, hiddenState, m.eps)
	return m.Output.Forward(ctx, hiddenState)
}

func newTextModel(c ml.Config) *TextModel {
	var decoderLayers []TextDecoderLayer
	for i := range c.Uint("block_count") {
		var textDecoderLayer TextDecoderLayer
		if slices.Contains(c.Uints("attention.cross_attention_layers"), i) {
			textDecoderLayer = &TextCrossAttentionDecoderLayer{}
		} else {
			textDecoderLayer = &TextSelfAttentionDecoderLayer{}
		}

		decoderLayers = append(decoderLayers, textDecoderLayer)
	}

	return &TextModel{
		Transformer: &TextDecoder{Layers: decoderLayers},
		TextModelOptions: &TextModelOptions{
			hiddenSize:           int64(c.Uint("embedding_length")),
			numHeads:             int64(c.Uint("attention.head_count")),
			numKVHeads:           int64(c.Uint("attention.head_count_kv")),
			eps:                  c.Float("attention.layer_norm_rms_epsilon"),
			ropeBase:             c.Float("rope.freq_base"),
			ropeScale:            c.Float("rope.freq_scale", 1),
			ropeDim:              c.Uint("rope.dimension_count"),
			crossAttentionLayers: c.Uints("attention.cross_attention_layers"),
		},
	}
}