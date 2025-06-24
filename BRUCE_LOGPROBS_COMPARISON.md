# Comparison: Bruce MacDonald's vs Our Log Probabilities Implementation

## Overview

After examining Bruce MacDonald's `brucemacd/logprobs` branch, here are the key differences and insights from his approach compared to our implementation.

## Key Differences

### 1. API Structure

**Bruce's Approach:**
- Added `LogProbs int` field to `GenerateRequest` and `ChatRequest` (specifies number of log probs to return)
- Uses `TokenProbs` struct with fields:
  ```go
  type TokenProbs struct {
      TokenID int     `json:"id"`
      LogProb float32 `json:"logprob"`
      Token   string  `json:"token"`
  }
  ```
- Returns `LogProbs []TokenProbs` in `ChatResponse` and `GenerateResponse`
- Does NOT modify the OpenAI compatibility layer

**Our Approach:**
- Added `LogProbs bool` and `TopLogProbs *int` to `ChatCompletionRequest` in OpenAI layer
- Created more complex structures to match OpenAI's format:
  ```go
  type LogProbs struct {
      Content []LogProbContent `json:"content"`
  }
  type LogProbContent struct {
      Token       string       `json:"token"`
      LogProb     float32      `json:"logprob"`
      Bytes       []byte       `json:"bytes,omitempty"`
      TopLogProbs []LogProbToken `json:"top_logprobs,omitempty"`
  }
  ```
- Modified both Ollama native API and OpenAI compatibility layer

### 2. Implementation Scope

**Bruce's Approach:**
- Focused on Ollama's native API only
- Simpler, more direct implementation
- No OpenAI compatibility layer modifications
- Returns token ID alongside the token text

**Our Approach:**
- Comprehensive implementation covering both APIs
- Added OpenAI-compatible request/response structures
- More complex conversion logic between formats
- Focus on OpenAI schema compatibility

### 3. LLM Server Integration

**Bruce's Approach:**
- Modified `llm/server.go` to include `LogProbs int` in `CompletionRequest`
- Returns `LogProbs []TokenProbs` in `CompletionResponse`
- Simpler token probability structure in the completion response

**Our Approach:**
- Added `n_probs` parameter to llama.cpp requests
- More complex parsing of `completion_probabilities` from llama.cpp
- Conversion logic to transform llama.cpp format to OpenAI format

### 4. Token Information

**Bruce's Approach:**
- Includes `TokenID` in the response (useful for debugging and analysis)
- Simpler structure makes it easier to process

**Our Approach:**
- Focuses on OpenAI compatibility
- Includes byte representation of tokens
- Supports top-k log probabilities for each token

## Lessons Learned from Bruce's Implementation

### 1. **Simplicity First**
Bruce's implementation is notably simpler and more focused. He chose to:
- Keep the native Ollama API separate from OpenAI compatibility
- Use a minimal structure that provides essential information
- Avoid complex conversions and nested structures

### 2. **Token IDs are Valuable**
Bruce includes token IDs in the response, which our implementation doesn't. This is useful for:
- Debugging tokenization issues
- Understanding model behavior
- Correlating with vocabulary files

### 3. **Incremental Approach**
Bruce's implementation doesn't try to solve everything at once:
- No OpenAI compatibility layer changes
- Focus on core functionality first
- Leaves room for future enhancements

### 4. **Native API Design**
Bruce's approach suggests that Ollama's native API should have its own design philosophy rather than trying to mirror OpenAI exactly.

## Recommendations

Based on Bruce's approach, we might consider:

1. **Simplifying our native API implementation** - Use Bruce's simpler `TokenProbs` structure for Ollama's native API
2. **Including Token IDs** - Add token IDs to our response for better debugging capabilities
3. **Separating concerns** - Keep OpenAI compatibility as a separate layer rather than mixing it with native API
4. **Phased approach** - Consider implementing log probabilities in phases:
   - Phase 1: Native API support (like Bruce's)
   - Phase 2: OpenAI compatibility layer
   - Phase 3: Advanced features (top-k, bytes, etc.)

## Technical Implementation Details

### Bruce's Server Route Handler
```go
// Simplified log probability handling
for _, p := range r.LogProbs {
    res.LogProbs = append(res.LogProbs, api.TokenProbs{
        TokenID: p.TokenID,
        LogProb: p.LogProb,
        Token:   p.Token,
    })
}
```

### Our Implementation
```go
// Complex conversion with top-k support
topK := int(3)
logits := make([]float32, len(cr.Logits))
copy(logits, cr.Logits)
res.TopLogprobs = getTopKLogProbs(c.Request.Context(), r, logits, topK)
```

## Conclusion

Bruce MacDonald's implementation demonstrates a more idiomatic approach for Ollama:
- Simpler and more maintainable
- Focuses on core functionality
- Doesn't conflate native API with OpenAI compatibility
- Provides useful debugging information (token IDs)

While our implementation provides fuller OpenAI compatibility, Bruce's approach suggests that starting simple and building incrementally might be a better strategy for the official Ollama codebase.