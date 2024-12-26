# Check if Go is already installed
if command -v go &> /dev/null
then
    echo "Go is already installed."
    go version
    exit 1
fi

# Check if wget is installed
if ! command -v wget &> /dev/null
then
    echo "wget could not be found. Please install wget to use this script."
    exit 1
fi

# Check if tar is installed
if ! command -v tar &> /dev/null
then
    echo "tar could not be found. Please install tar to use this script."
    exit 1
fi

# get latest if GO_TAR_URL variable is not set
if [ -z ${GO_TAR_URL+x} ]
then
    # fetches the latest version of go from the official website
    export GO_LATEST_VERSION=$(curl -s https://go.dev/VERSION?m=text | head -n 1)
    export GO_TAR_URL="https://go.dev/dl/${GO_LATEST_VERSION}.linux-amd64.tar.gz"
    echo "GO_TAR_URL is not set. Fetching the latest version: $GO_TAR_URL"
else
    echo "Fetching $GO_TAR_URL"
fi

# Download Go tarball
wget $GO_TAR_URL -P /tmp

# Extract Go tarball from temp (use filename from URL)
sudo tar -C /usr/local -xzf /tmp/$(basename $GO_TAR_URL)

# Create a new directories for Go workspace
mkdir -p $HOME/go/{bin,src,pkg}

# Specify bash source file to add Go environment variables
BASHRC=$HOME/.bashrc

echo $BASHRC

# Add Go environment variables to .bashrc
echo -e "\n# Load Go environment variables" >> $BASHRC
echo -e "export GOPATH=\$HOME/go" >> $BASHRC
echo -e "export GOBIN=\$HOME/go/bin" >> $BASHRC
echo -e "export PATH=\$PATH:/usr/local/go/bin:\$GOPATH/bin" >> $BASHRC

bash
