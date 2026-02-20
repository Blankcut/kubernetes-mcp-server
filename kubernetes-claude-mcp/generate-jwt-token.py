#!/usr/bin/env python3

import jwt
import base64
import time
import json

# ArgoCD server secret key (base64 decoded)
secret_key = base64.b64decode("wLkdBHcW62QUTMyiw8jI9C9Z7lXLmxVzj0kL6WqSJQ4=")

# Token ID from the existing token
token_id = "df87de36-8a1d-42a8-afd8-f803284b0927"

# Current time
current_time = int(time.time())

# JWT payload
payload = {
    "iss": "argocd",
    "sub": "mcp-server-account:apiKey",
    "nbf": current_time,
    "iat": current_time,
    "jti": token_id
}

# Generate JWT token
token = jwt.encode(payload, secret_key, algorithm="HS256")

print(f"Generated JWT token: {token}")
print(f"\nUpdate your config.yaml with:")
print(f"argocd:")
print(f"  authToken: \"{token}\"")
