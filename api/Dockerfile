FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/kernelquest-api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache docker-cli ca-certificates
WORKDIR /app
COPY --from=build /out/kernelquest-api /app/kernelquest-api
# The trusted control plane needs access to the Docker socket. Incident containers
# never receive this image, its mount, or its credentials.
EXPOSE 8080
ENTRYPOINT ["/app/kernelquest-api"]
