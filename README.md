# Terraform Provider for Ceph

This Terraform provider allows you to manage Ceph cluster resources including pools, users, and RBD block images.
Please note that this is under development, to be considered experimental, and in no way are you assumed to be a sane person if you use this code in its current state.
This project is a collaboration between Mike Burkhart and Chris Williams, as an attempt to vibecode a TF provider into existence that has some functionality.

## Features

- **Pool Management**: Create, update, and delete Ceph pools with configurable properties
- **User Management**: Manage Ceph authentication users with specific capabilities
- **RBD Block Images**: Create and manage RADOS Block Device images
- **Data Sources**: Query cluster status and pool information

## Requirements

- Terraform >= 1.0
- Go >= 1.21 (for building)
- Ceph cluster with admin access
- `ceph` and `rbd` command-line tools installed

## Installation

### Building from Source

1. Clone the repository:
```bash
git clone https://github.com/your-org/terraform-provider-ceph.git
cd terraform-provider-ceph
```

2. Build and install:
```bash
make install
```

### Using Pre-built Binaries

Download the appropriate binary for your platform from the releases page and place it in your Terraform plugins directory.

## Configuration

The provider supports the following configuration options:

```hcl
provider "ceph" {
  config_file = "/etc/ceph/ceph.conf"    # Path to Ceph config file
  keyring     = "/etc/ceph/ceph.client.admin.keyring"  # Path to keyring file
  user        = "admin"                   # Ceph user name
}
```

All configuration options are optional and will use Ceph defaults if not specified.

## Resources

### ceph_pool

Manages a Ceph pool.

```hcl
resource "ceph_pool" "example" {
  name       = "my-pool"
  pg_num     = 128
  pgp_num    = 128
  size       = 3
  min_size   = 2
  type       = "replicated"
  crush_rule = "replicated_rule"
}
```

#### Arguments

- `name` (Required) - Pool name
- `pg_num` (Required) - Number of placement groups
- `pgp_num` (Optional) - Number of placement groups for placement (defaults to pg_num)
- `size` (Optional) - Replication size
- `min_size` (Optional) - Minimum replication size
- `type` (Optional) - Pool type: "replicated" or "erasure"
- `crush_rule` (Optional) - CRUSH rule name

### ceph_user

Manages a Ceph authentication user.

```hcl
resource "ceph_user" "example" {
  name = "client.myapp"
  caps = {
    mon = "allow r"
    osd = "allow rw pool=mypool"
    mds = "allow rw"
  }
}
```

#### Arguments

- `name` (Required) - User name (e.g., "client.myapp")
- `caps` (Required) - Map of daemon types to capabilities

#### Attributes

- `key` - The generated authentication key

### ceph_block_image

Manages a RADOS Block Device image.

```hcl
resource "ceph_block_image" "example" {
  name     = "my-image"
  pool     = "rbd"
  size     = "10G"
  features = ["layering", "exclusive-lock"]
}
```

#### Arguments

- `name` (Required) - Image name
- `pool` (Required) - Pool name where the image will be created
- `size` (Required) - Image size (e.g., "10G", "1T")
- `features` (Optional) - List of RBD features to enable

## Data Sources

### ceph_cluster_status

Retrieves Ceph cluster status information.

```hcl
data "ceph_cluster_status" "cluster" {}
```

#### Attributes

- `health` - Cluster health status
- `osd_count` - Number of OSDs
- `mon_count` - Number of monitors
- `mgr_count` - Number of managers
- `pool_count` - Number of pools

### ceph_pool

Retrieves information about an existing pool.

```hcl
data "ceph_pool" "existing" {
  name = "rbd"
}
```

#### Arguments

- `name` (Required) - Pool name

#### Attributes

- `pg_num` - Number of placement groups
- `size` - Replication size
- `min_size` - Minimum replication size
- `type` - Pool type

## Examples

See the `examples/` directory for complete configuration examples.

### Basic Pool and User Setup

```hcl
terraform {
  required_providers {
    ceph = {
      source = "local/ceph/ceph"
      version = "0.1.0"
    }
  }
}

provider "ceph" {
  config_file = "/etc/ceph/ceph.conf"
  keyring     = "/etc/ceph/ceph.client.admin.keyring"
}

resource "ceph_pool" "app_data" {
  name    = "app-data"
  pg_num  = 64
  size    = 3
  min_size = 2
}

resource "ceph_user" "app_user" {
  name = "client.app"
  caps = {
    mon = "allow r"
    osd = "allow rw pool=${ceph_pool.app_data.name}"
  }
}
```

## Development

### Building

```bash
go build -o terraform-provider-ceph
```

### Testing

```bash
make test
```

### Running Acceptance Tests

```bash
make testacc
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For issues and questions:
- Create an issue on GitHub
- Check the Ceph documentation for cluster-specific questions

## Changelog

### v0.1.0
- Initial release
- Basic pool, user, and block image management
- Cluster status and pool data sources
