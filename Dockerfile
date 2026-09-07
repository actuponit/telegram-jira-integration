# Multi-stage build for Cloud Run. Static binary, distroless runtime,
# non-root. The assignee mapping ships inside the image because it is
# hand-edited and redeploy-to-change (ADR-0001).
FROM golang:1.24 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/bot /app/bot
COPY configs/ /app/configs/

ENV ASSIGNEE_MAPPING_PATH=/app/configs/assignees.yaml
ENV PORT=8080
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/bot"]
