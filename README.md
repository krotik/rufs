Rufs
====
Rufs is a remote union filesystem which aims to provide a lightweight and secure solution for distributed file storage. Rufs uses a client-server system where servers expose branches and clients mount one or several branches into a tree structure. The client can overlay branches providing a union view.

<p>
<a href="https://void.devt.de/pub/rufs/coverage.txt"><img src="https://void.devt.de/pub/rufs/test_result.svg" alt="Code coverage"></a>
<a href="https://goreportcard.com/report/devt.de/krotik/rufs">
<img src="https://goreportcard.com/badge/devt.de/krotik/rufs?style=flat-square" alt="Go Report Card"></a>
<a href="https://godoc.org/devt.de/krotik/rufs">
<img src="https://godoc.org/devt.de/krotik/rufs?status.svg" alt="Go Doc"></a>
</p>

Features
--------
- Client-Server model using RPC call over SSL.
- Single executable for client and server.
- Communication is secured via a secret token which is never transferred over the network and certificate pinning once a client has connected successfully.
- Clients can provide a unified view with files from different locations.
- Default client provides CLI, REST API and a web interface.
- Branches can be read-only.
- A read-only version of the file system can be exported via FUSE and mounted.

Getting Started
---------------
You can download a pre-compiled package for Windows (win64) or Linux (amd64) [here](https://void.devt.de/pub/rufs).

The archive contains a single executable which contains the server and client code for Rufs.

You can also pull the latest docker image of Rufs from [Dockerhub](https://hub.docker.com/r/krotik/rufs):
```
docker pull krotik/rufs
```

Create an empty directory, change into it and run the following to start the server:
```
docker run --rm --user $(id -u):$(id -g) -v $PWD:/data -p 9020:9020 krotik/rufs server
```
This exposes port 9020 from the container on the local machine. All runtime related files are written to the current directory as the current user/group.

Run the client by running:
```
docker run --rm --network="host" -it -v $PWD:/data --user $(id -u):$(id -g) -v $PWD:/data krotik/rufs client
```
The client will also use the runtime related files from the current directory.

### Tutorial:

To get an idea of what Rufs is about have a look at the [tutorial](https://devt.de/krotik/rufs/src/master/examples/tutorial/doc/tutorial.md).

### REST API:

The terminal uses a REST API to communicate with the backend. The REST API can be browsed using a dynamically generated swagger.json definition (https://localhost:9090/fs/swagger.json). You can browse the API of Rufs's latest version [here](http://petstore.swagger.io/?url=https://devt.de/krotik/rufs/raw/master/swagger.json).

### Command line options
The main Rufs executable has two main tools:
```
Rufs 1.0.0

Usage of ./rufs [tool]

The tools are:

    server    Run as a server
    client    Run as a client

Use ./rufs [tool] --help for more information about a tool.
```
The most important one is `server` which starts the file server. The server has several options:
```
Rufs 1.0.0

Usage of ./rufs server [options]

  -config string
    	Server configuration file (default "rufs.server.json")
  -help
    	Show this help message
  -secret string
    	Secret file containing the secret token (default "rufs.secret")
  -ssl-dir string
    	Directory containing the ssl key.pem and cert.pem files (default "ssl")

The server will automatically create a default config file and
default directories if nothing is specified.
```
Once the server is started the client tool can be used to interact with the server. The options of the client tool are:
```
Rufs 1.0.0

Usage of ./rufs client [mapping file]

  -fuse-mount string
    	Mount tree as FUSE filesystem at specified path (read-only)
  -help
    	Show this help message
  -secret string
    	Secret file containing the secret token (default "rufs.secret")
  -ssl-dir string
    	Directory containing the ssl key.pem and cert.pem files (default "ssl")
  -web string
    	Export the tree through a https interface on the specified host:port

The mapping file assignes remote branches to the local tree.
The client tries to load rufs.mapping.json if no mapping file is defined.
It starts empty if no mapping file exists. The mapping file
should have the following json format:

{
  "branches" : [
    {
      "branch"      : <branch name>,
      "rpc"         : <rpc interface>,
      "fingerprint" : <fingerprint>
    },
    ...
  ],
  "tree" : [
    {
      "path"      : <path>,
      "branch"    : <branch name>,
      "writeable" : <writable flag>
    },
    ...
  ]
}
```
On the console type `q` to exit and `help` to get an overview of available commands:
```
Available commands:
----
branch [branch name] [rpc] [fingerprint] : List all known branches or add a new branch to the tree
cat <file>                               : Read and print the contents of a file
cd [path]                                : Show or change the current directory
checksum [path] [glob]                   : Show a directory listing and file checksums
cp <src file/dir> <dst dir>              : Copy a file or directory
dir [path] [glob]                        : Show a directory listing
get <src file> [dst local file]          : Retrieve a file and store it locally (in the current directory)
help [cmd]                               : Show general or command specific help
mkdir <dir>                              : Create a new directory
mount [path] [branch name] [ro]          : List all mount points or add a new mount point to the tree
ping <branch name> [rpc]                 : Ping a remote branch
put [src local file] [dst file]          : Read a local file and store it
refresh                                  : Refreshes all known branches and reconnect if possible.
ren <file> <newfile>                     : Rename a file or directory
reset [mounts|brances]                   : Remove all mounts or all mounts and all branches
rm <file>                                : Delete a file or directory (* all files; ** all files/recursive)
storeconfig [local file]                 : Store the current tree mapping in a local file
sync <src dir> <dst dir>                 : Make sure dst has the same files and directories as src
tree [path] [glob]                       : Show the listing of a directory and its subdirectories
```

### Configuration
The Rufs client and server use each their own configuration file and require a shared `rufs.secret` file to be able to talk to each other. The server configuration is called `rufs.server.json`. After starting the server for the first time it should create a default configuration file. Available configurations are:

| Configuration Option | Description |
| --- | --- |
| BranchName | Branch name which the server will export. |
| EnableReadOnly | Export the branch only for read operations. |
| LocalFolder | Local physical folder which is exported. |
| RPCHost | RPC host for communication with clients. |
| RPCPort | RPC port for communication with clients. |

Note: It is not (and will never be) possible to access the REST API via HTTP.

Building Rufs
----------------
To build Rufs from source you need to have Go installed (go >= 1.12):

Create a directory, change into it and run:
```
git clone https://devt.de/krotik/rufs/ .
```

You can build Rufs's executable with:
```
go build -o rufs ./cli/...
```

Rufs also has a web interface which should be bundled with the executable. The bundled web interface in `web.zip` can be attached by running:
```
./attach_webzip.sh
```
This assumes that the `rufs` executable is in the same folder as the script.

Building Rufs as Docker image
--------------------------------
Rufs can be build as a secure and compact Docker image.

- Create a directory, change into it and run:
```
git clone https://devt.de/krotik/rufs/ .
```

- You can now build the Docker image with:
```
docker build --tag krotik/rufs .
```

License
-------
Rufs source code is available under the [MIT License](/LICENSE).
