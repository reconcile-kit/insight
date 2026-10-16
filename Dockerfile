# Frontend
FROM node:22-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY index.html vite.config.js tailwind.config.js postcss.config.js ./
COPY src ./src
COPY public ./public
RUN npm run build

# Backend with the frontend embedded
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/insight ./cmd/insight

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/insight /insight
EXPOSE 27182
ENTRYPOINT ["/insight"]
