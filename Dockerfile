FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o vfmp

FROM scratch AS prod

WORKDIR /app

COPY --from=builder /build/vfmp .

ENTRYPOINT [ "./vfmp" ]
