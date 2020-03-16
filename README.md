# 1. Install operator-sdk
```ecma script level 4
https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md
```

### 1.1 Install from GitHub release
Download the release binary
```ecma script level 4
# Set the release version variable
$ RELEASE_VERSION=v0.15.2
# Linux
$ curl -LO https://github.com/operator-framework/operator-sdk/releases/download/${RELEASE_VERSION}/operator-sdk-${RELEASE_VERSION}-x86_64-linux-gnu
# macOS
$ curl -LO https://github.com/operator-framework/operator-sdk/releases/download/${RELEASE_VERSION}/operator-sdk-${RELEASE_VERSION}-x86_64-apple-darwin
```

### 1.2 Install the release binary in your PATH
```ecma script level 4
# Linux
$ chmod +x operator-sdk-${RELEASE_VERSION}-x86_64-linux-gnu && sudo mkdir -p /usr/local/bin/ && sudo cp operator-sdk-${RELEASE_VERSION}-x86_64-linux-gnu /usr/local/bin/operator-sdk && rm operator-sdk-${RELEASE_VERSION}-x86_64-linux-gnu
# macOS
$ chmod +x operator-sdk-${RELEASE_VERSION}-x86_64-apple-darwin && sudo mkdir -p /usr/local/bin/ && sudo cp operator-sdk-${RELEASE_VERSION}-x86_64-apple-darwin /usr/local/bin/operator-sdk && rm operator-sdk-${RELEASE_VERSION}-x86_64-apple-darwin
```

ls

