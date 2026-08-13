FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /payment-api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /payment-worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /payment-api /payment-api
COPY --from=build /payment-worker /payment-worker
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/payment-api"]
