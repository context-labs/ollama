# Log Probabilities Implementation for Ollama

## Overview

This implementation adds support for log probabilities in Ollama's OpenAI-compatible chat completion endpoints. When enabled, the API returns token-level probability information alongside the generated text, which is useful for:

- Confidence scoring
- Token-level analysis
- Alternative token exploration
- Perplexity calculations
- Model behavior analysis

## API Usage

### Request Parameters

The following parameters control log probabilities in the OpenAI-compatible chat completions endpoint (`/v1/chat/completions`):

- `logprobs` (boolean): Whether to return log probabilities. Default: `false`
- `top_logprobs` (integer): Number of most likely tokens to return at each position (0-5). Default: `0`

### Example Request

```json
{
  "model": "llama3.2:latest",
  "messages": [
    {"role": "user", "content": "Hello, how are you?"}
  ],
  "logprobs": true,
  "top_logprobs": 3,
  "stream": false
}
```

### Response Format

When log probabilities are enabled, the response includes a `logprobs` field in each choice:

```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "llama3.2:latest",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "I'm doing well, thank you!"
    },
    "logprobs": {
      "content": [
        {
          "token": "I",
          "logprob": -0.123,
          "bytes": [73],
          "top_logprobs": [
            {"token": "I", "logprob": -0.123, "bytes": [73]},
            {"token": "Hello", "logprob": -2.456, "bytes": [72, 101, 108, 108, 111]},
            {"token": "Hi", "logprob": -3.789, "bytes": [72, 105]}
          ]
        },
        {
          "token": "'m",
          "logprob": -0.456,
          "bytes": [39, 109],
          "top_logprobs": [
            {"token": "'m", "logprob": -0.456, "bytes": [39, 109]},
            {"token": " am", "logprob": -1.234, "bytes": [32, 97, 109]},
            {"token": "'ve", "logprob": -4.567, "bytes": [39, 118, 101]}
          ]
        }
        // ... more tokens
      ]
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 6,
    "total_tokens": 16
  }
}
```

### Streaming Response

When streaming is enabled (`"stream": true`), log probabilities are included in each chunk:

```json
data: {
  "id": "chatcmpl-123",
  "object": "chat.completion.chunk",
  "created": 1234567890,
  "model": "llama3.2:latest",
  "choices": [{
    "index": 0,
    "delta": {"content": "Hello"},
    "logprobs": {
      "content": [{
        "token": "Hello",
        "logprob": -0.123,
        "bytes": [72, 101, 108, 108, 111],
        "top_logprobs": [...]
      }]
    }
  }]
}
```

## Implementation Details

### Architecture

The implementation spans several key components:

1. **OpenAI Compatibility Layer** (`openai/openai.go`):
   - Added `LogProbs` and `TopLogProbs` fields to `ChatCompletionRequest`
   - Created log probability structures (`LogProbs`, `LogProbContent`, `LogProbToken`)
   - Updated response conversion functions to include log probabilities

2. **API Types** (`api/types.go`):
   - Added `LogProb` and `TopLogProb` structs to the internal API
   - Extended `Message` struct with `LogProbs` field

3. **LLM Server Interface** (`llm/server.go`):
   - Updated `CompletionRequest` to include log probability parameters
   - Added log probability fields to completion response structures
   - Modified the completion request to llama.cpp to include `n_probs` parameter

4. **Request Handler** (`server/routes.go`):
   - Updated `ChatHandler` to extract log probability settings from context
   - Modified completion request to include log probability parameters
   - Added conversion logic from llama.cpp format to OpenAI format

### Data Flow

1. Client sends request with `logprobs=true` and `top_logprobs=N`
2. OpenAI middleware extracts these parameters and stores them in gin context
3. ChatHandler retrieves parameters and includes them in the completion request
4. LLM server adds `n_probs` parameter when calling llama.cpp
5. llama.cpp returns `completion_probabilities` in its response
6. Response is converted from llama.cpp format to OpenAI format
7. Log probabilities are included in the final response to the client

### Format Conversion

The implementation converts between two formats:

**llama.cpp format**:
```json
{
  "completion_probabilities": [{
    "content": "token",
    "probs": [
      {"tok_str": "token1", "prob": -0.123},
      {"tok_str": "token2", "prob": -1.456}
    ]
  }]
}
```

**OpenAI format**:
```json
{
  "logprobs": {
    "content": [{
      "token": "token1",
      "logprob": -0.123,
      "bytes": [116, 111, 107, 101, 110, 49],
      "top_logprobs": [...]
    }]
  }
}
```

## Testing

A test script (`test_logprobs.py`) is provided to verify the implementation:

```bash
# Test with default model
python test_logprobs.py

# Test with specific model
python test_logprobs.py mistral:latest
```

The test script:
- Verifies both streaming and non-streaming responses
- Displays token-level probabilities
- Shows top alternative tokens when available
- Validates the response format matches OpenAI's schema

## Limitations

1. **Top Log Probabilities**: The current implementation depends on llama.cpp's `n_probs` parameter. If llama.cpp doesn't return multiple probabilities per token, only the selected token's probability will be shown.

2. **Token Bytes**: The implementation converts tokens to UTF-8 bytes. This may not perfectly match OpenAI's tokenization for all models.

3. **Model Support**: Log probabilities require support from the underlying llama.cpp server. Some models or server versions may not support this feature.

## Future Enhancements

1. **Caching**: Consider caching log probabilities for repeated requests
2. **Performance**: Optimize the format conversion for large responses
3. **Validation**: Add more robust validation for the `top_logprobs` parameter
4. **Extended Support**: Support log probabilities in other endpoints (e.g., completions)