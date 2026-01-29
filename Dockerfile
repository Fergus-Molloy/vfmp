FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -o vfmp -ldflags "-X fergus.molloy.xyz/vfmp/internal/version.Version=$VERSION"

FROM scratch AS prod

WORKDIR /app

COPY --from=builder /build/vfmp .

ENTRYPOINT [ "/app/vfmp" ]
