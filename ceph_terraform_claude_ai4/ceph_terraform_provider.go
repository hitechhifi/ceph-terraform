package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Provider definition
type cephProvider struct{}

type cephProviderModel struct {
	ConfigFile types.String `tfsdk:"config_file"`
	Keyring    types.String `tfsdk:"keyring"`
	User       types.String `tfsdk:"user"`
}

func New() provider.Provider {
	return &cephProvider{}
}

func (p *cephProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ceph"
}

func (p *cephProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for managing Ceph cluster resources",
		Attributes: map[string]schema.Attribute{
			"config_file": schema.StringAttribute{
				Description: "Path to Ceph configuration file",
				Optional:    true,
			},
			"keyring": schema.StringAttribute{
				Description: "Path to Ceph keyring file",
				Optional:    true,
			},
			"user": schema.StringAttribute{
				Description: "Ceph user name",
				Optional:    true,
			},
		},
	}
}

func (p *cephProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config cephProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := &CephClient{
		ConfigFile: config.ConfigFile.ValueString(),
		Keyring:    config.Keyring.ValueString(),
		User:       config.User.ValueString(),
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *cephProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPoolResource,
		NewUserResource,
		NewBlockImageResource,
	}
}

func (p *cephProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterStatusDataSource,
		NewPoolDataSource,
	}
}

// Ceph client
type CephClient struct {
	ConfigFile string
	Keyring    string
	User       string
}

func (c *CephClient) buildCmdArgs(cmd string) []string {
	args := strings.Split(cmd, " ")
	if c.ConfigFile != "" {
		args = append(args, "--conf", c.ConfigFile)
	}
	if c.Keyring != "" {
		args = append(args, "--keyring", c.Keyring)
	}
	if c.User != "" {
		args = append(args, "--user", c.User)
	}
	return args
}

func (c *CephClient) ExecuteCommand(cmd string) (string, error) {
	args := c.buildCmdArgs(cmd)
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return "", fmt.Errorf("command failed: %w", err)
	}
	return string(out), nil
}

// Pool Resource
type poolResource struct {
	client *CephClient
}

type poolResourceModel struct {
	Name       types.String `tfsdk:"name"`
	PgNum      types.Int64  `tfsdk:"pg_num"`
	PgpNum     types.Int64  `tfsdk:"pgp_num"`
	Size       types.Int64  `tfsdk:"size"`
	MinSize    types.Int64  `tfsdk:"min_size"`
	Type       types.String `tfsdk:"type"`
	CrushRule  types.String `tfsdk:"crush_rule"`
}

func NewPoolResource() resource.Resource {
	return &poolResource{}
}

func (r *poolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *poolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Ceph pool",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Pool name",
				Required:    true,
			},
			"pg_num": schema.Int64Attribute{
				Description: "Placement group number",
				Required:    true,
			},
			"pgp_num": schema.Int64Attribute{
				Description: "Placement group for placement number",
				Optional:    true,
			},
			"size": schema.Int64Attribute{
				Description: "Pool replication size",
				Optional:    true,
			},
			"min_size": schema.Int64Attribute{
				Description: "Pool minimum replication size",
				Optional:    true,
			},
			"type": schema.StringAttribute{
				Description: "Pool type (replicated or erasure)",
				Optional:    true,
			},
			"crush_rule": schema.StringAttribute{
				Description: "CRUSH rule name",
				Optional:    true,
			},
		},
	}
}

func (r *poolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*CephClient)
}

func (r *poolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan poolResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolType := "replicated"
	if !plan.Type.IsNull() {
		poolType = plan.Type.ValueString()
	}

	cmd := fmt.Sprintf("ceph osd pool create %s %d %d %s",
		plan.Name.ValueString(),
		plan.PgNum.ValueInt64(),
		plan.PgpNum.ValueInt64(),
		poolType)

	_, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create pool", err.Error())
		return
	}

	// Set pool properties
	if !plan.Size.IsNull() {
		cmd = fmt.Sprintf("ceph osd pool set %s size %d",
			plan.Name.ValueString(), plan.Size.ValueInt64())
		_, err = r.client.ExecuteCommand(cmd)
		if err != nil {
			resp.Diagnostics.AddError("Failed to set pool size", err.Error())
			return
		}
	}

	if !plan.MinSize.IsNull() {
		cmd = fmt.Sprintf("ceph osd pool set %s min_size %d",
			plan.Name.ValueString(), plan.MinSize.ValueInt64())
		_, err = r.client.ExecuteCommand(cmd)
		if err != nil {
			resp.Diagnostics.AddError("Failed to set pool min_size", err.Error())
			return
		}
	}

	if !plan.CrushRule.IsNull() {
		cmd = fmt.Sprintf("ceph osd pool set %s crush_rule %s",
			plan.Name.ValueString(), plan.CrushRule.ValueString())
		_, err = r.client.ExecuteCommand(cmd)
		if err != nil {
			resp.Diagnostics.AddError("Failed to set crush rule", err.Error())
			return
		}
	}

	tflog.Info(ctx, "Created Ceph pool", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *poolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state poolResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("ceph osd pool get %s all", state.Name.ValueString())
	output, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read pool", err.Error())
		return
	}

	// Parse output to update state
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "size:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				size, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				state.Size = types.Int64Value(size)
			}
		}
		if strings.Contains(line, "min_size:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				minSize, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				state.MinSize = types.Int64Value(minSize)
			}
		}
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *poolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan poolResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update pool properties
	if !plan.Size.IsNull() {
		cmd := fmt.Sprintf("ceph osd pool set %s size %d",
			plan.Name.ValueString(), plan.Size.ValueInt64())
		_, err := r.client.ExecuteCommand(cmd)
		if err != nil {
			resp.Diagnostics.AddError("Failed to update pool size", err.Error())
			return
		}
	}

	if !plan.MinSize.IsNull() {
		cmd := fmt.Sprintf("ceph osd pool set %s min_size %d",
			plan.Name.ValueString(), plan.MinSize.ValueInt64())
		_, err := r.client.ExecuteCommand(cmd)
		if err != nil {
			resp.Diagnostics.AddError("Failed to update pool min_size", err.Error())
			return
		}
	}

	tflog.Info(ctx, "Updated Ceph pool", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *poolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state poolResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("ceph osd pool delete %s %s --yes-i-really-really-mean-it",
		state.Name.ValueString(), state.Name.ValueString())
	_, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete pool", err.Error())
		return
	}

	tflog.Info(ctx, "Deleted Ceph pool", map[string]interface{}{
		"name": state.Name.ValueString(),
	})
}

// User Resource
type userResource struct {
	client *CephClient
}

type userResourceModel struct {
	Name     types.String `tfsdk:"name"`
	Caps     types.Map    `tfsdk:"caps"`
	Key      types.String `tfsdk:"key"`
}

func NewUserResource() resource.Resource {
	return &userResource{}
}

func (r *userResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Ceph user",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "User name",
				Required:    true,
			},
			"caps": schema.MapAttribute{
				Description: "User capabilities",
				ElementType: types.StringType,
				Required:    true,
			},
			"key": schema.StringAttribute{
				Description: "User key (computed)",
				Computed:    true,
			},
		},
	}
}

func (r *userResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*CephClient)
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build caps string
	capsMap := make(map[string]string)
	diags = plan.Caps.ElementsAs(ctx, &capsMap, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var capsArgs []string
	for daemon, caps := range capsMap {
		capsArgs = append(capsArgs, daemon, caps)
	}

	cmd := fmt.Sprintf("ceph auth get-or-create %s %s",
		plan.Name.ValueString(), strings.Join(capsArgs, " "))
	
	output, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user", err.Error())
		return
	}

	// Extract key from output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "key =") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				plan.Key = types.StringValue(strings.TrimSpace(parts[1]))
			}
		}
	}

	tflog.Info(ctx, "Created Ceph user", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("ceph auth get %s", state.Name.ValueString())
	output, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		if strings.Contains(err.Error(), "entity does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read user", err.Error())
		return
	}

	// Parse output to verify user exists
	if !strings.Contains(output, state.Name.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build caps string
	capsMap := make(map[string]string)
	diags = plan.Caps.ElementsAs(ctx, &capsMap, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var capsArgs []string
	for daemon, caps := range capsMap {
		capsArgs = append(capsArgs, daemon, caps)
	}

	cmd := fmt.Sprintf("ceph auth caps %s %s",
		plan.Name.ValueString(), strings.Join(capsArgs, " "))
	
	_, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update user caps", err.Error())
		return
	}

	tflog.Info(ctx, "Updated Ceph user", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("ceph auth del %s", state.Name.ValueString())
	_, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete user", err.Error())
		return
	}

	tflog.Info(ctx, "Deleted Ceph user", map[string]interface{}{
		"name": state.Name.ValueString(),
	})
}

// Block Image Resource
type blockImageResource struct {
	client *CephClient
}

type blockImageResourceModel struct {
	Name     types.String `tfsdk:"name"`
	Pool     types.String `tfsdk:"pool"`
	Size     types.String `tfsdk:"size"`
	Features types.Set    `tfsdk:"features"`
}

func NewBlockImageResource() resource.Resource {
	return &blockImageResource{}
}

func (r *blockImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_block_image"
}

func (r *blockImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Ceph RBD block image",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Image name",
				Required:    true,
			},
			"pool": schema.StringAttribute{
				Description: "Pool name",
				Required:    true,
			},
			"size": schema.StringAttribute{
				Description: "Image size (e.g., 10G, 1T)",
				Required:    true,
			},
			"features": schema.SetAttribute{
				Description: "RBD features",
				ElementType: types.StringType,
				Optional:    true,
			},
		},
	}
}

func (r *blockImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*CephClient)
}

func (r *blockImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan blockImageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("rbd create --size %s %s/%s",
		plan.Size.ValueString(),
		plan.Pool.ValueString(),
		plan.Name.ValueString())

	if !plan.Features.IsNull() {
		var features []string
		diags = plan.Features.ElementsAs(ctx, &features, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		
		if len(features) > 0 {
			cmd += " --image-feature " + strings.Join(features, ",")
		}
	}

	_, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create block image", err.Error())
		return
	}

	tflog.Info(ctx, "Created Ceph block image", map[string]interface{}{
		"name": plan.Name.ValueString(),
		"pool": plan.Pool.ValueString(),
	})

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *blockImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state blockImageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("rbd info %s/%s --format json",
		state.Pool.ValueString(),
		state.Name.ValueString())
	
	output, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read block image", err.Error())
		return
	}

	var imageInfo map[string]interface{}
	if err := json.Unmarshal([]byte(output), &imageInfo); err != nil {
		resp.Diagnostics.AddError("Failed to parse image info", err.Error())
		return
	}

	// Update size from actual image
	if size, ok := imageInfo["size"].(float64); ok {
		state.Size = types.StringValue(fmt.Sprintf("%.0fB", size))
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *blockImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan blockImageResourceModel
	var state blockImageResourceModel
	
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update size if changed
	if !plan.Size.Equal(state.Size) {
		cmd := fmt.Sprintf("rbd resize --size %s %s/%s",
			plan.Size.ValueString(),
			plan.Pool.ValueString(),
			plan.Name.ValueString())
		
		_, err := r.client.ExecuteCommand(cmd)
		if err != nil {
			resp.Diagnostics.AddError("Failed to resize block image", err.Error())
			return
		}
	}

	tflog.Info(ctx, "Updated Ceph block image", map[string]interface{}{
		"name": plan.Name.ValueString(),
		"pool": plan.Pool.ValueString(),
	})

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *blockImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state blockImageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := fmt.Sprintf("rbd rm %s/%s",
		state.Pool.ValueString(),
		state.Name.ValueString())
	
	_, err := r.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete block image", err.Error())
		return
	}

	tflog.Info(ctx, "Deleted Ceph block image", map[string]interface{}{
		"name": state.Name.ValueString(),
		"pool": state.Pool.ValueString(),
	})
}

// Cluster Status Data Source
type clusterStatusDataSource struct {
	client *CephClient
}

type clusterStatusDataSourceModel struct {
	Health     types.String `tfsdk:"health"`
	OSDCount   types.Int64  `tfsdk:"osd_count"`
	MonCount   types.Int64  `tfsdk:"mon_count"`
	MGRCount   types.Int64  `tfsdk:"mgr_count"`
	PoolCount  types.Int64  `tfsdk:"pool_count"`
}

func NewClusterStatusDataSource() datasource.DataSource {
	return &clusterStatusDataSource{}
}

func (d *clusterStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_status"
}

func (d *clusterStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Ceph cluster status data source",
		Attributes: map[string]schema.Attribute{
			"health": schema.StringAttribute{
				Description: "Cluster health status",
				Computed:    true,
			},
			"osd_count": schema.Int64Attribute{
				Description: "Number of OSDs",
				Computed:    true,
			},
			"mon_count": schema.Int64Attribute{
				Description: "Number of monitors",
				Computed:    true,
			},
			"mgr_count": schema.Int64Attribute{
				Description: "Number of managers",
				Computed:    true,
			},
			"pool_count": schema.Int64Attribute{
				Description: "Number of pools",
				Computed:    true,
			},
		},
	}
}

func (d *clusterStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*CephClient)
}

func (d *clusterStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state clusterStatusDataSourceModel

	// Get cluster status
	output, err := d.client.ExecuteCommand("ceph status --format json")
	if err != nil {
		resp.Diagnostics.AddError("Failed to get cluster status", err.Error())
		return
	}

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		resp.Diagnostics.AddError("Failed to parse cluster status", err.Error())
		return
	}

	// Parse health
	if health, ok := status["health"].(map[string]interface{}); ok {
		if healthStatus, ok := health["status"].(string); ok {
			state.Health = types.StringValue(healthStatus)
		}
	}

	// Parse service counts
	if services, ok := status["servicemap"].(map[string]interface{}); ok {
		if services, ok := services["services"].(map[string]interface{}); ok {
			if osd, ok := services["osd"].(map[string]interface{}); ok {
				if daemons, ok := osd["daemons"].(map[string]interface{}); ok {
					state.OSDCount = types.Int64Value(int64(len(daemons)))
				}
			}
			if mon, ok := services["mon"].(map[string]interface{}); ok {
				if daemons, ok := mon["daemons"].(map[string]interface{}); ok {
					state.MonCount = types.Int64Value(int64(len(daemons)))
				}
			}
			if mgr, ok := services["mgr"].(map[string]interface{}); ok {
				if daemons, ok := mgr["daemons"].(map[string]interface{}); ok {
					state.MGRCount = types.Int64Value(int64(len(daemons)))
				}
			}
		}
	}

	// Get pool count
	poolOutput, err := d.client.ExecuteCommand("ceph osd pool ls")
	if err == nil {
		pools := strings.Split(strings.TrimSpace(poolOutput), "\n")
		state.PoolCount = types.Int64Value(int64(len(pools)))
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Pool Data Source
type poolDataSource struct {
	client *CephClient
}

type poolDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	PgNum   types.Int64  `tfsdk:"pg_num"`
	Size    types.Int64  `tfsdk:"size"`
	MinSize types.Int64  `tfsdk:"min_size"`
	Type    types.String `tfsdk:"type"`
}

func NewPoolDataSource() datasource.DataSource {
	return &poolDataSource{}
}

func (d *poolDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *poolDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Ceph pool data source",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Pool name",
				Required:    true,
			},
			"pg_num": schema.Int64Attribute{
				Description: "Placement group number",
				Computed:    true,
			},
			"size": schema.Int64Attribute{
				Description: "Pool replication size",
				Computed:    true,
			},
			"min_size": schema.Int64Attribute{
				Description: "Pool minimum replication size",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Pool type",
				Computed:    true,
			},
		},
	}
}

func (d *poolDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*CephClient)
}

func (d *poolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config poolDataSourceModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get pool information
	cmd := fmt.Sprintf("ceph osd pool get %s all", config.Name.ValueString())
	output, err := d.client.ExecuteCommand(cmd)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get pool information", err.Error())
		return
	}

	var state poolDataSourceModel
	state.Name = config.Name

	// Parse pool properties
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "size:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				size, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				state.Size = types.Int64Value(size)
			}
		}
		if strings.Contains(line, "min_size:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				minSize, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				state.MinSize = types.Int64Value(minSize)
			}
		}
		if strings.Contains(line, "pg_num:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				pgNum, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				state.PgNum = types.Int64Value(pgNum)
			}
		}
	}

	// Get pool type
	cmd = fmt.Sprintf("ceph osd pool get %s type", config.Name.ValueString())
	output, err = d.client.ExecuteCommand(cmd)
	if err == nil {
		parts := strings.Split(output, ":")
		if len(parts) == 2 {
			poolType := strings.TrimSpace(parts[1])
			state.Type = types.StringValue(poolType)
		}
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Main function
func main() {
	provider.Serve(context.Background(), provider.ServeOpts{
		ProviderFunc: New,
	})
}