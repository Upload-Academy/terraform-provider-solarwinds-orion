package orion

import (
	"context"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mrxinu/gosolar"
)

type orion struct {
	version string
}

type orionConfig struct {
	Server types.String `tfsdk:"server"`
	// Port types.String `tfsdk:"port"`
	Insecure types.Bool   `tfsdk:"insecure"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// This essentials creates an unused variable to ensure that the provider.Provider interface is implemented
var _ provider.Provider = &orion{}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &orion{
			version: version,
		}
	}
}

func (p *orion) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "orion"
}

func (p *orion) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"server": schema.StringAttribute{
				Required: true,
			},
			// "port": schema.StringAttribute{
			//     Optional: true,
			//     // Default: "17778",
			// },
			"insecure": schema.BoolAttribute{
				Optional: true,
				// Default: false,
			},
			"username": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
			},
		},
	}
}

func (p *orion) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config orionConfig

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Server.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("server"),
			"Unknown value for server address given",
			"Failed to create SolarWinds Orion client with given server address",
		)
	}

	if config.Insecure.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("insecure"),
			"Unknown value for insecure flag given",
			"Failed to create SolarWinds Orion client with given insecure flag",
		)
	}

	// if config.Port.IsUnknown() {
	//     resp.Diagnostics.AddAttributeError(
	//         path.Root("port"),
	//         "Unknown value for port given",
	//         "Failed to create SolarWinds Orion client with given port",
	//     )
	// }

	if config.Username.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Unknown value for username given",
			"Failed to create SolarWinds Orion client with given username",
		)
	}

	if config.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Unknown vaue for password given",
			"Failed to create SolarWinds Orion client with given password",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	server := os.Getenv("SOLARWINDS_ORION_SERVER")
	// port := os.Getenv("SOLARWINDS_ORION_PORT")

	insecure, err := strconv.ParseBool(os.Getenv("SOLARWINDS_ORION_INSECURE"))
	if err != nil {
		panic(nil)
	}

	username := os.Getenv("SOLARWINDS_ORION_USERNAME")
	password := os.Getenv("SOLARWINDS_ORION_PASSWORD")

	if !config.Server.IsNull() {
		server = config.Server.ValueString()
	}

	// if !config.Port.IsNull() {
	//     port = config.Port.ValueString()
	// }

	if !config.Insecure.IsNull() {
		insecure = config.Insecure.ValueBool()
	}

	if !config.Username.IsNull() {
		username = config.Username.ValueString()
	}

	if !config.Password.IsNull() {
		password = config.Password.ValueString()
	}

	if server == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("server"),
			"No server address given",
			"Failed to create SolarWinds Orion client as no server address was given",
		)
	}

	if username == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"No username given",
			"Failed to create SolarWinds Orion client as no username was given",
		)
	}

	if password == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"No password given",
			"Failed to create SolarWinds Orion client as no password was given",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	client := gosolar.NewClient(server, username, password, insecure)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *orion) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *orion) Resources(_ context.Context) []func() resource.Resource {
	return nil
}
