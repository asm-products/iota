# iota
On demand stateless compute service


### Installing
- Probably only works on Linux; there's a bash script that likely isn't cross-platform
- `git clone URI` into the "proper" place, mine is in ~/go/src/github.com/asm-products/iota
- The git project has some submodules; `git submodule update --init`
- GOPATH needs to include the src directory and the vendor diretory  (mine is 
  `export GOPATH=$HOME/go/src/github.com/asm-products/iota/vendor:$HOME/go`)
- `go build` in the project root (the directory with iota.go)
- `./iota`

### URLs
- localhost:8000/{user}/{package}/{filename}/src
- localhost:8000/{user}/{package}/f/{functionName}

Samples are at /testuser/hello/hello.go/src and /testuser/hello/f/Hello. If you want
to add a new user, you will need to manually add a directory (and src dir) in the 
user folder.

### Caveats
- This is **extremely insecure**; should definetly not be publicly accessible
- You have to save the source to start the service. If you stop iota, it loses track
  of what endpoints are available, and won't be able to reach the endpoint anymore.
  Go the src page and rebuild.

