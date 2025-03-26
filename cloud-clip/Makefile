


#  test -z "`git diff-index --name-only HEAD --`" || VN="$$VN-d";
define GIT_VER
 VN=`git describe --tags --match "go-[0-9]*" | sed -e 's/-[0-9]-/-/'`;
 test -z "`git diff-index --name-only HEAD -- *.go`" || VN="$$VN-d";
 echo $$VN
endef

# VER=$(shell git describe --tags --match "go-[0-9]*" | sed -e 's/-[0-9]-/-/')
# VER=$(shell ${GIT_VER})
VER=$(shell \
 VN=`git describe --tags --match "go-[0-9]*" | sed -e 's/-[0-9]-/-/'`; \
 test -z "`git diff-index --name-only HEAD -- *.go`" || VN="$$VN-d"; \
 echo $$VN \
)

## tag=`git tag --points-at HEAD --sort=-refname  | head -n1`
## git describe --tags --dirty --always --match $tag

VER=$(shell \
 VN=`git tag --points-at HEAD --sort=-refname  | head -n1`; \
 test -z "`git diff-index --name-only HEAD -- *.go`" || VN="$$VN-d"; \
 echo $$VN \
)

FLAGS= -ldflags='-s -w -X main.server_version=${VER}' 


cloud-clip: *.go Makefile
	go build -o $@ $(FLAGS)

# bundle with static/
cloud-clip.embed: *.go Makefile
	go build -o $@ $(FLAGS) --tags embed

clean:
	rm -f cloud-clip
