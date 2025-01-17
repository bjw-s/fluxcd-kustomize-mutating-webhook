FROM golang:1.23 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o main .

FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /app/main .

ENTRYPOINT ["./main"]
