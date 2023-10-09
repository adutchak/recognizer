docker-build-push:
	docker build --platform=linux/amd64 -t adutchak/recognizer:$(tag) .
	docker push adutchak/recognizer:$(tag)
docker-build-latest:
	docker build --platform=linux/amd64 -t adutchak/recognizer:latest .
