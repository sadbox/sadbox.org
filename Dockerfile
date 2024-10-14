FROM golang:1.23
WORKDIR /sadbox.org
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
ADD views ./views
ADD static-files ./static-files
COPY *.go .
#RUN ls -lah /db/sadbot_archive.db && exit 1
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -v -o /bin/sadbox.org .
CMD ["/bin/sadbox.org"]
