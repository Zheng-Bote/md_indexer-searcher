---
type: security-module
name: TokenSigner
algorithm: Ed25519
key_rotation: 90d
---
# Token Signer Module

Cryptographic module responsible for signing and verifying asymmetric authentication tokens.

## Implementation Details

Implements standard RFC 8032 Ed25519 signatures.
Utilized exclusively by [[AuthService]].

```go
func SignToken(claims map[string]interface{}) (string, error)
```

## Security Requirements
- Keys stored in Hardware Security Module (HSM)
- Fast verification (<1ms)
