#!/bin/bash

# Generate ArgoCD token for mcp-server-account

echo "Setting up port-forward to ArgoCD server..."
kubectl port-forward -n argocd svc/argocd-server 8083:80 &
PORT_FORWARD_PID=$!

# Wait for port-forward to be ready
sleep 5

echo "Getting admin password..."
ADMIN_PASSWORD=$(kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath="{.data.password}" | base64 -d)

echo "Logging in to ArgoCD..."
argocd login localhost:8083 --username admin --password "$ADMIN_PASSWORD" --insecure

echo "Generating token for mcp-server-account..."
TOKEN=$(argocd account generate-token --account mcp-server-account --server localhost:8083)

echo "Generated token: $TOKEN"

# Clean up
kill $PORT_FORWARD_PID

echo "Token generation complete!"
echo "Update your config.yaml with this token:"
echo "argocd:"
echo "  authToken: \"$TOKEN\""
