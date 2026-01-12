# Start with a small Go image
FROM golang:1.21-alpine

WORKDIR /app

# Copy ALL files into the container
COPY . .

# This is the magic line that fixes your error:
# It downloads dependencies and generates the missing go.sum file inside Docker
RUN go mod tidy

# Build only the API server entrypoint
RUN go build -o main ./main.go

# Run the binary
CMD ["./main"]