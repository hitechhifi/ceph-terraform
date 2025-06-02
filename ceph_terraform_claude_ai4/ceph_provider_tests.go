package main

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ceph": providerserver.NewProtocol6WithError(New()),
}

func TestAccCephPoolResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccCephPoolResourceConfig("test-pool", 32, 32, 3, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_pool.test", "name", "test-pool"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_num", "32"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pgp_num", "32"),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "3"),
					resource.TestCheckResourceAttr("ceph_pool.test", "min_size", "2"),
				),
			},
			// Update and Read testing
			{
				Config: testAccCephPoolResourceConfig("test-pool", 32, 32, 2, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_pool.test", "name", "test-pool"),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "2"),
					resource.TestCheckResourceAttr("ceph_pool.test", "min_size", "1"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccCephPoolResourceConfig(name string, pgNum, pgpNum, size, minSize int) string {
	return fmt.Sprintf(`
resource "ceph_pool" "test" {
  name     = %[1]q
  pg_num   = %[2]d
  pgp_num  = %[3]d
  size     = %[4]d
  min_size = %[5]d
}
`, name, pgNum, pgpNum, size, minSize)
}

func TestAccCephUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccCephUserResourceConfig("client.test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_user.test", "name", "client.test"),
					resource.TestCheckResourceAttrSet("ceph_user.test", "key"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccCephUserResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "ceph_user" "test" {
  name = %[1]q
  caps = {
    mon = "allow r"
    osd = "allow rw pool=rbd"
  }
}
`, name)
}

func TestAccCephBlockImageResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccCephBlockImageResourceConfig("test-image", "rbd", "1G"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_block_image.test", "name", "test-image"),
					resource.TestCheckResourceAttr("ceph_block_image.test", "pool", "rbd"),
					resource.TestCheckResourceAttr("ceph_block_image.test", "size", "1G"),
				),
			},
			// Update and Read testing
			{
				Config: testAccCephBlockImageResourceConfig("test-image", "rbd", "2G"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_block_image.test", "size", "2G"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccCephBlockImageResourceConfig(name, pool, size string) string {
	return fmt.Sprintf(`
resource "ceph_block_image" "test" {
  name = %[1]q
  pool = %[2]q
  size = %[3]q
}
`, name, pool, size)
}

func TestAccCephClusterStatusDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccCephClusterStatusDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.ceph_cluster_status.test", "health"),
					resource.TestCheckResourceAttrSet("data.ceph_cluster_status.test", "osd_count"),
					resource.TestCheckResourceAttrSet("data.ceph_cluster_status.test", "mon_count"),
				),
			},
		},
	})
}

func testAccCephClusterStatusDataSourceConfig() string {
	return `
data "ceph_cluster_status" "test" {}
`
}

func TestAccCephPoolDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccCephPoolDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_pool.test", "name", "rbd"),
					resource.TestCheckResourceAttrSet("data.ceph_pool.test", "pg_num"),
					resource.TestCheckResourceAttrSet("data.ceph_pool.test", "size"),
				),
			},
		},
	})
}

func testAccCephPoolDataSourceConfig() string {
	return `
data "ceph_pool" "test" {
  name = "rbd"
}
`
}

// Unit tests for CephClient
func TestCephClient_buildCmdArgs(t *testing.T) {
	tests := []struct {
		name       string
		client     *CephClient
		cmd        string
		expected   []string
	}{
		{
			name:     "basic command",
			client:   &CephClient{},
			cmd:      "ceph status",
			expected: []string{"ceph", "status"},
		},
		{
			name: "with config file",
			client: &CephClient{
				ConfigFile: "/etc/ceph/ceph.conf",
			},
			cmd:      "ceph status",
			expected: []string{"ceph", "status", "--conf", "/etc/ceph/ceph.conf"},
		},
		{
			name: "with keyring",
			client: &CephClient{
				Keyring: "/etc/ceph/ceph.client.admin.keyring",
			},
			cmd:      "ceph status",
			expected: []string{"ceph", "status", "--keyring", "/etc/ceph/ceph.client.admin.keyring"},
		},
		{
			name: "with user",
			client: &CephClient{
				User: "admin",
			},
			cmd:      "ceph status",
			expected: []string{"ceph", "status", "--user", "admin"},
		},
		{
			name: "with all options",
			client: &CephClient{
				ConfigFile: "/etc/ceph/ceph.conf",
				Keyring:    "/etc/ceph/ceph.client.admin.keyring",
				User:       "admin",
			},
			cmd:      "ceph status",
			expected: []string{"ceph", "status", "--conf", "/etc/ceph/ceph.conf", "--keyring", "/etc/ceph/ceph.client.admin.keyring", "--user", "admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.client.buildCmdArgs(tt.cmd)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d args, got %d", len(tt.expected), len(result))
				return
			}
			for i, arg := range result {
				if arg != tt.expected[i] {
					t.Errorf("expected arg %d to be %q, got %q", i, tt.expected[i], arg)
				}
			}
		})
	}
}

// Integration test helper functions
func testAccPreCheck(t *testing.T) {
	// Add any pre-check requirements here
	// For example, check if Ceph cluster is available
}

// Benchmark tests
func BenchmarkCephClient_buildCmdArgs(b *testing.B) {
	client := &CephClient{
		ConfigFile: "/etc/ceph/ceph.conf",
		Keyring:    "/etc/ceph/ceph.client.admin.keyring",
		User:       "admin",
	}
	cmd := "ceph osd pool create test 32 32"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.buildCmdArgs(cmd)
	}
}