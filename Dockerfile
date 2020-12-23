FROM golang:latest

WORKDIR /usr/src/app

COPY . .

RUN go build -o jia ./cmd

CMD ["./jia"]
