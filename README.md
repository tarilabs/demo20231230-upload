# Project demo20231230-upload

Experimenting with ORAS and Minio/S3

## Notes
oapi-codegen --config server.cfg.yaml api/openapi.yaml

kubectl apply -k config/secrets

kubectl port-forward svc/minio 9090
kubectl port-forward svc/minio 9000 

