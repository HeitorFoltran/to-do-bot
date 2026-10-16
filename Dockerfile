FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/merge-bot ./cmd/bot \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/merge-bot /merge-bot
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV DB_PATH=/data/merges.db
VOLUME ["/data"]
ENTRYPOINT ["/merge-bot"]
