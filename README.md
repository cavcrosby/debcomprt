# debcomprt

Partially some copy pasta from debian/control. Currently **debcomprt** is a
program that creates debian compartments. A debian compartment is a chrooted
'target' generated from debootstrap but with added configuration for the target
post creation.

By default configuration is done via a shell script with ```make``` substituting in
specific shell variables. The shell scripts used currently by debcomprt come
from the following [repository](https://github.com/cavcrosby/comprtconfigs).
That said, the shell script can also come from a local shell script on your
filesystem (via --config-path).

# Getting Started

## Installation

### Debian && Debian-like distros

```shell
echo "deb https://cavcrosby.jfrog.io/artifactory/deb/ bullseye main" | sudo tee "/etc/apt/sources.list.d/cavcrosby-artifactory.list"
wget --quiet --output-document - "https://cavcrosby.jfrog.io/artifactory/api/gpg/key/public" | gpg --dearmor | dd of="/etc/apt/trusted.gpg.d/cavcrosby-artifactory.gpg"
apt-get update
apt-get install debcomprt
```

## Usage Examples

```shell
sudo debcomprt create \
    --config-path comprtconfig \
    --crypt-password '$6$1234$B4Ea6.5maChxdHpoCfkYFc8J7w8gfAowUfyjuATHSPawgcRn.jJExi6itqEhSMJLGdwTxKRlHX1XZ/SmDufT0."' \
    buster foo
```
This example creates a Debian base system in the directory ```foo```. The comprt
configuration is designated by the option value to --config-path and the default
comprt user is created with the password passed in by the option value to
--crypt-password. Finally, ```buster``` is the version of Debian installed in
```foo```.

```shell
sudo debcomprt chroot foo
```
Following from the previous example, debcomprt has functionality built into it
to chroot into a directory. Assuming the directory was a created comprt,
debcomprt will proceed to chroot into the target directory and login as the
default comprt user.

# Tree Versioning Policy

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
