# Start by building the application.
FROM golang:latest as build

WORKDIR /go/src/github.com/nixmade/orchestrator
COPY . .

RUN apt-get install -y ca-certificates
ENV GO111MODULE=on
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-extldflags '-static'" -o /go/bin/orchestrator cmd/orchestrator/main.go

# Now copy it into our base image.
FROM gcr.io/distroless/base-debian12
COPY --from=build /go/bin/orchestrator /