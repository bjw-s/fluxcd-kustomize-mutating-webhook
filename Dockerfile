FROM golang:1.22 as builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go test -v ./...

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/webhook

FROM gcr.io/distroless/base-debian12

WORKDIR /

COPY --from=builder /app/main .

USER nonroot:nonroot

ENTRYPOINT ["./main"]
