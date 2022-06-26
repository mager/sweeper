
dev:
	go mod tidy && go run main.go

test:
	go test ./...

build:
	gcloud builds submit --tag gcr.io/floorreport/sweeper

deploy:
	gcloud run deploy sweeper \
		--image gcr.io/floorreport/sweeper \
		--platform managed

ship:
	make test && make build && make deploy