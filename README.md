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

## Tree Versioning Policy

1.  Any changes to files under debian/* directory will result in the debian_revision
    being incremented. Thus after the commit the latest tag will to be moved to 
    the recent commit (the resulting uploaded package will only have the
    debian_revision incremented).
2.  Any changes to the project will either increment the major.minor.patch version
    number. What this means is best left to interpretation as to what counts as a
    change to the project (e.g. changing the README should not require incrementing
    the patch version).

## License

See LICENSE.
