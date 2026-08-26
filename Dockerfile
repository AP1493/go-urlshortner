FROM golang:1.26-alpine

WORKDIR /app

COPY go.mod ./

RUN go mod download

COPY . .

RUN go build -o server cmd/main.go

EXPOSE 8080

CMD ["./server"]
