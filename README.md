# debcomprt

Partially some copy pasta from debian/control. Currently debcomprt is a
program that creates debian compartments. A debian compartment is a chrooted
'target' generated from debootstrap but with added configuration for the target
post creation.

## Installation

### Debian && Debian-like distros

```shell
echo "deb https://cavcrosby.jfrog.io/artifactory/deb/ bullseye main" | sudo tee "/etc/apt/sources.list.d/cavcrosby-artifactory.list"
wget --quiet --output-document - "https://cavcrosby.jfrog.io/artifactory/api/gpg/key/public" | gpg --dearmor | dd of="/etc/apt/trusted.gpg.d/cavcrosby-artifactory.gpg"
apt-get update
apt-get install debcomprt
```

## License

See LICENSE.
