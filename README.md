# static-file-server

> **🔀 FORK with upload support** — this is a fork of
> [halverneus/static-file-server](https://github.com/halverneus/static-file-server)
> that adds **file upload via `PUT`/`POST`** (see
> [Uploads](#uploads-put--post) section below). Built and published to GHCR as
> `ghcr.io/josemzr/static-file-server`.

<img src="img/sponsor.svg" />

## Introduction

Tiny, simple static file server using environment variables for configuration.
Install from any of the following locations:

- Docker Hub: https://hub.docker.com/r/halverneus/static-file-server/
- GitHub: https://github.com/halverneus/static-file-server

## Configuration

### Environment Variables

Default values are shown with the associated environment variable.

```bash
# Enables resource access from any domain.
CORS=false

# Enable debugging for troubleshooting. If set to 'true' this prints extra
# information during execution. IMPORTANT NOTE: The configuration summary is
# printed to stdout while logs generated during execution are printed to stderr.
DEBUG=false

# Optional Hostname for binding. Leave unset to accept any incoming HTTP request
# on the prescribed port.
HOST=

# If assigned, must be a valid port number.
PORT=8080

# When set to 'true' the index.html file in the folder will be served. And
# the file list will not be served.
ALLOW_INDEX=true

# Automatically serve the index of file list for a given directory (default).
SHOW_LISTING=true

# Folder with the content to serve.
FOLDER=/web

# Subdirectory (relative to $FOLDER) where PUT and POST requests may upload
# files. Uploads are streamed to disk and subdirectories are created on
# demand. Set to an empty string to disable uploads entirely.
UPLOAD_DIR=uploads

# URL path prefix. If 'my.file' is in the root of $FOLDER and $URL_PREFIX is
# '/my/place' then file is retrieved with 'http://$HOST:$PORT/my/place/my.file'.
URL_PREFIX=

# Paths to the TLS certificate and key. If one is set then both must be set. If
# both set then files are served using HTTPS. If neither are set then files are
# served using HTTP.
TLS_CERT=
TLS_KEY=

# If TLS certificates are set then the minimum TLS version may also be set. If
# the value isn't set then the default minimum TLS version is 1.0. Allowed
# values include "TLS10", "TLS11", "TLS12" and "TLS13" for TLS1.0, TLS1.1,
# TLS1.2 and TLS1.3, respectively. The value is not case-sensitive.
TLS_MIN_VERS=

# List of accepted HTTP referrers. Return 403 if HTTP header `Referer` does not
# match prefixes provided in the list.
# Examples:
#   'REFERRERS=http://localhost,https://...,https://another.name'
#   To accept missing referrer header, add a blank entry (start comma):
#   'REFERRERS=,http://localhost,https://another.name'
REFERRERS=

# Use key / code parameter in the request URL for access control. The code is
# computed by requested PATH and your key.
# Example:
#   ACCESS_KEY=username
#   To access your file, either access:
#   http://$HOST:$PORT/my/place/my.file?key=username
#   or access (md5sum of "/my/place/my.fileusername"):
#   http://$HOST:$PORT/my/place/my.file?code=44356A355E89D9EE7B2D5687E48024B0
ACCESS_KEY=
```

### YAML Configuration File

YAML settings are individually overridden by the corresponding environment
variable. The following is an example configuration file with defaults. Pass in
the path to the configuration file using the command line option
('-c', '-config', '--config').

```yaml
cors: false
debug: false
folder: /web
host: ""
port: 8080
referrers: []
show-listing: true
tls-cert: ""
tls-key: ""
tls-min-vers: ""
url-prefix: ""
access-key: ""
```

Example configuration with possible alternative values:

```yaml
debug: true
folder: /var/www
port: 80
referrers:
    - http://localhost
    - https://mydomain.com
```

## Deployment

### Without Docker

```bash
PORT=8888 FOLDER=. ./serve
```

Files can then be accessed by going to http://localhost:8888/my/file.txt

### With Docker

```bash
docker run -d \
    -v /my/folder:/web \
    -p 8080:8080 \
    halverneus/static-file-server:latest
```

This will serve the folder "/my/folder" over http://localhost:8080/my/file.txt

Any of the variables can also be modified:

```bash
docker run -d \
    -v /home/me/dev/source:/content/html \
    -v /home/me/dev/files:/content/more/files \
    -e FOLDER=/content \
    -p 8080:8080 \
    halverneus/static-file-server:latest
```

### Getting Help

```bash
./serve help
# OR
docker run -it halverneus/static-file-server:latest help
```

## Uploads (PUT / POST)

This fork adds file upload support. When `UPLOAD_DIR` is set (default:
`uploads`), `PUT` and `POST` requests write the request body into
`$FOLDER/$UPLOAD_DIR`. Subdirectories are created on demand, and paths are
sanitized so uploads can never escape the upload directory (path traversal
is rejected).

Uploads can be addressed with or without the upload prefix in the URL:

```
PUT /uploads/file.txt  -> $FOLDER/uploads/file.txt
PUT /file.txt          -> $FOLDER/uploads/file.txt
```

### With curl

```bash
curl --upload-file myfile.bin http://localhost:8080/uploads/myfile.bin
# or equivalently:
curl -X PUT --data-binary @myfile.bin http://localhost:8080/myfile.bin
```

Uploaded files are served back immediately, e.g.
`http://localhost:8080/uploads/myfile.bin`.

### Docker example

```bash
docker run -d \
    -v /home/me/data/upload-folder:/web \
    -e FOLDER=/web \
    -e UPLOAD_DIR=uploads \
    -p 8080:8080 \
    ghcr.io/josemzr/static-file-server:latest
```

Set `UPLOAD_DIR=` (empty) to disable uploads entirely. Note: this feature is
intended for trusted, private networks — there is no authentication, so do
not expose it to the public internet without a reverse proxy or access key.
