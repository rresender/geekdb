FROM golang
RUN go get github.com/hashicorp/serf/serf
RUN go get github.com/pkg/errors
RUN go get github.com/gorilla/mux
RUN go get golang.org/x/sync/errgroup
RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN go clean
RUN go build -o geek_db_main *.go
CMD ["/app/geek_db_main"]
