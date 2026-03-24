.PHONY: build garmin strava keep test clean

build:
	go build -o bin/garmin_sync ./cmd/garmin_sync
	go build -o bin/strava_sync ./cmd/strava_sync
	go build -o bin/keep_sync ./cmd/keep_sync

garmin:
	go run ./cmd/garmin_sync $(ARGS)

strava:
	go run ./cmd/strava_sync $(ARGS)

keep:
	go run ./cmd/keep_sync $(ARGS)

test:
	go test ./...

clean:
	rm -rf bin/
