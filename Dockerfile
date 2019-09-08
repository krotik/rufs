# Start from the latest alpine based golang base image
FROM golang:alpine as builder

# Install git
RUN apk update && apk add --no-cache git

# Add maintainer info
LABEL maintainer="Matthias Ladkau <matthias@ladkau.de>"

# Set the current working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source from the current directory to the working directory inside the container
COPY . .

# Build rufs and link statically (no CGO)
# Use ldflags -w -s to omit the symbol table, debug information and the DWARF table
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags="-w -s" -o rufs ./cli/...
RUN sh ./attach_webzip.sh

# Start again from scratch
FROM scratch

# Copy the rufs binary
COPY --from=builder /app/rufs /rufs

# Set the working directory to data so all created files (e.g. rufs.config.json)
# can be mapped to physical files on disk
WORKDIR /data

# Run eliasdb binary
ENTRYPOINT ["../rufs"]

# To run the server as the current user, expose port 9020 and preserve 
# all runtime related files on disk in the local directory run:
#
# docker run --rm --user $(id -u):$(id -g) -v $PWD:/data -p 9020:9020 krotik/rufs server

# To run the client CLI as the current user and use the rufs.secret in the local directory run:

# docker run --rm --network="host" -it -v $PWD:/data --user $(id -u):$(id -g) -v $PWD:/data krotik/rufs client
