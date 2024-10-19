FROM node:23-alpine3.19 as node-builder
WORKDIR /app
COPY package.json yarn.lock ./
RUN yarn install
COPY tailwind.config.js ./
COPY views ./views
RUN yarn run tailwindcss -o tailwind.css

FROM golang:1.23 as golang-builder
WORKDIR /sadbox.org
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
COPY *.go .
#RUN ls -lah /db/sadbot_archive.db && exit 1
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -v -o /bin/sadbox.org .

FROM ubuntu:24.04
WORKDIR /bin
ADD views /views
ADD static-files /static-files
COPY --from=node-builder /app/node_modules/jquery/dist/jquery.min.js /static-files/vendor/jquery.min.js
COPY --from=node-builder /app/node_modules/jquery/dist/jquery.min.map /static-files/vendor/jquery.min.js.map
COPY --from=node-builder /app/node_modules/chart.js/dist/chart.umd.js /static-files/vendor/chart.umd.js
COPY --from=node-builder /app/node_modules/chart.js/dist/chart.umd.js.map /static-files/vendor/chart.umd.js.map
COPY --from=node-builder /app/node_modules/date-fns/cdn.min.js /static-files/vendor/date-fns.min.js
COPY --from=node-builder /app/node_modules/date-fns/cdn.min.js.map /static-files/vendor/date-fns.min.js.map
COPY --from=node-builder /app/node_modules/chartjs-adapter-date-fns/dist/chartjs-adapter-date-fns.bundle.min.js /static-files/vendor/chartjs-adapter-date-fns.bundle.min.js
COPY --from=node-builder /app/tailwind.css /static-files/static/tailwind.css
COPY --from=golang-builder /bin/sadbox.org ./
CMD ["/bin/sadbox.org"]
