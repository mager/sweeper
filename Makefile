
dev:
	go run main.go

test:
	go test ./...

build:
	gcloud builds submit --tag gcr.io/floor-report-327113/sweeper

deploy:
	gcloud run deploy sweeper \
		--image gcr.io/floor-report-327113/sweeper \
		--platform managed

ship:
	make test && make build && make deploy