package orion

import (
	"context"
	"fmt"
	"time"
    "strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/mrxinu/gosolar"
)

type Subnet struct {
	SubnetId      int    `json:"subnetid"`
	Uri           string `json:"uri"`
	CIDR          int    `json:"cidr"`
	GroupTypeText string `json:"grouptypetext"`
	Address       string `json:"address"`
	VlanName      string `json:"vlan"`
}

type IPEntity struct {
	IpNodeId  int    `json:"ipnodeid"`
	SubnetId  int    `json:"subnetid"`
	IPAddress string `json:"ipaddress"`
	Comments  string `json:"comments"`
	Status    int    `json:"status"`
	Uri       string `json:"uri"`
}

func NewIPResource() resource.Resource {
	return &resourceIP{}
}

type resourceIPReservationModel struct {
	ID          types.String `tfsdk:"id"`
	LastUpdated types.String `tfsdk:"last_updated"`

	VLANAddress    string `tfsdk:"vlan_address"`
	VLANName       string `tfsdk:"vlan_name"`
	VLANMask       int    `tfsdk:"vlan_mask"`
	Comment        string `tfsdk:"comment"`
	StatusCode     int    `tfsdk:"status_code"`
	IPAddress      string `tfsdk:"ip_address"`
	AvoidDHCPScope bool   `tfsdk:"avoid_dhcp_scope"`
}

type resourceIP struct {
	client *gosolar.Client
}

func (r *resourceIP) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip"
}

func (r *resourceIP) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(*gosolar.Client)
}

func (r *resourceIP) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"vlan_address": schema.StringAttribute{
				Required: true,
			},
			"vlan_name": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"vlan_mask": schema.NumberAttribute{
				Optional: true,
			},
			"comment": schema.StringAttribute{
				Required: true,
			},
			"status_code": schema.NumberAttribute{
				Optional: true,
			},
			"ip_address": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"avoid_dhcp_scope": schema.BoolAttribute{
				Optional: true,
			},
		},
	}
}

func (r *resourceIP) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewIPResource,
	}
}

func (r *resourceIP) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	client := r.client

	var plan resourceIPReservationModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	//declare vars
	vlan_address := plan.VLANAddress
	comment := plan.Comment
	avoid_dhcp_scope := plan.AvoidDHCPScope
	ip_address := plan.IPAddress
	status_code := plan.StatusCode
	vlan_name := plan.VLANName
	vlan_mask := plan.VLANMask

	computedVlanName, VlanNameErr := getVlanName(client, vlan_address)
	if VlanNameErr != nil {
		resp.Diagnostics.AddError(
			"Error creating IP reservation",
			VlanNameErr.Error(),
		)
		return
	}

	if vlan_name != "" && vlan_name != computedVlanName {
		resp.Diagnostics.AddError(
			"Error creating IP reservation",
			fmt.Sprintf("There is mismatch in vlan name that you've provided ('%s') and computed value which is %s", vlan_name, computedVlanName),
		)
		return
	}

	if vlan_name == "" {
		plan.VLANName = computedVlanName
	}

	subnetId, getSubnetErr := getSubnetId(client, vlan_address)
	if getSubnetErr != nil {
		// return getSubnetErr
		resp.Diagnostics.AddError(
			"Error creating IP reservation",
			getSubnetErr.Error(),
		)
		return
	}

	if avoid_dhcp_scope {
		subnetDHCP, getSubnetDhcpErr := checkIfSubnetDHCP(client, vlan_address)
		if getSubnetDhcpErr != nil {
			// return getSubnetDhcpErr
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				getSubnetDhcpErr.Error(),
			)
			return
		}

		if subnetDHCP && ip_address == "" {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				"No IP provided and avoid DHCP subnets is true, so cannot do anthing.",
			)
			return
		}

		if subnetDHCP && ip_address != "" {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				"You are trying to get static IP from subnet with DHCP Scope!",
			)
			return
		}
	}

	if ip_address == "" {
		ipEntity, getIpError := getFreeIpEntity(client, subnetId)
		if getIpError != nil {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				getIpError.Error(),
			)
			return
		}

		updateErr := updateIpEntity(client, *ipEntity, status_code, comment)
		if updateErr != nil {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				updateErr.Error(),
			)
			return
		}

		plan.IPAddress = ipEntity.IPAddress
		plan.ID = basetypes.NewStringValue(ipEntity.IPAddress)
		plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

		diags = resp.State.Set(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() {
			return
		}

	} else {
		ipError := validateAddresses(ip_address)
		if ipError != nil {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				ipError.Error(),
			)
			return
		}

		ipSubnetError := validateAddresInSubnet(vlan_address, vlan_mask, ip_address)
		if ipSubnetError != nil {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				ipSubnetError.Error(),
			)
			return
		}

		ipEntity, getIpError := getIpEntityByAddress(client, ip_address)
		if getIpError != nil {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				getIpError.Error(),
			)
			return
		}

		if ipEntity.Status != 2 {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				"You are trying to get IP that is already assigned!",
			)
			return
		}

		updateErr := updateIpEntity(client, *ipEntity, status_code, comment)
		if updateErr != nil {
			resp.Diagnostics.AddError(
				"Error creating IP reservation",
				updateErr.Error(),
			)
			return
		}

		plan.IPAddress = ipEntity.IPAddress
		plan.ID = basetypes.NewStringValue(ipEntity.IPAddress)
		plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

		diags = resp.State.Set(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() {
			return
		}
	}
}

func (r *resourceIP) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    client := r.client

    var state resourceIPReservationModel
    diags := resp.State.Get(ctx, &state)
    resp.Diagnostics.Append(diags...)
    if diags.HasError() {
        return
    }

	id := state.ID.String()
	vlan_address := state.VLANAddress
	comment := state.Comment
	avoid_dhcp_scope := state.AvoidDHCPScope
	ip_address := state.IPAddress
	vlan_name := state.VLANName

	//Validate if it's dhcp error to handle
	if id == "dhcp" && ip_address == "dhcp" {
		return 
	}

	computedVlanName, VlanNameErr := getVlanName(client, vlan_address)
	if VlanNameErr != nil {
        resp.Diagnostics.AddError(
            "Error reading IP reservation",
            VlanNameErr.Error(),
        )
        return
	}
	if vlan_name != "" && vlan_name != computedVlanName {
        resp.Diagnostics.AddError(
            "Error reading IP reservation",
            fmt.Sprintf("There is mismatch in vlan name that you've provided ('%s') and computed value which is %s", vlan_name, computedVlanName),
        )
        return
	}

	ipEntity,_ := getIpEntityByAddress(client, ip_address)

	//Validate if provided ip address is assigned to this machine
	if !strings.Contains(ipEntity.Comments, comment) && ipEntity.Status != 2 {
        resp.Diagnostics.AddError(
            "Error reading IP reservation",
            fmt.Sprintf("IP address '%s' is not assigned to '%s'", ip_address, comment),
        )
        return
	}

	dhcpScope,dhcpErr := checkIfSubnetDHCP(client, vlan_address)
	if dhcpErr != nil {
        resp.Diagnostics.AddError(
            "Error reading IP reservation",
            dhcpErr.Error(),
        )
        return
	}

	//Validate if subnet is DHCP AND avoid_dhcp_scope is true
	if avoid_dhcp_scope && dhcpScope {
        resp.Diagnostics.AddError(
            "Error reading IP reservation",
            "avoid_dhcp_flag set to true, but subnet HAS dhcp scope",
        ) 
        return
	}

    state.IPAddress = ipEntity.IPAddress

    diags = resp.State.Set(ctx, &state)
    resp.Diagnostics.Append(diags...)
    if diags.HasError() {
        return
    }
}

func (r *resourceIP) Update(_ context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

func (r *resourceIP) Delete(_ context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

//func resourceIp() *schema.Resource {
//	return &schema.Resource{
//		Create: resourceIpCreate,
//		Read:   resourceIpRead,
//		Update: resourceIpUpdate,
//		Delete: resourceIpDelete,
//		Importer: &schema.ResourceImporter{
//			State: resourceIpImport,
//		},
//		Schema: map[string]*schema.Schema{
//			"vlan_address": {
//				Type		: 	schema.TypeString,
//				Description	: 	"Vlan address",
//				Required	:	true,
//			},
//			"vlan_name": {
//				Type		:	schema.TypeString,
//				Description	: 	"Vlan name",
//				Optional	:	true,
//				Computed	:	true,
//			},
//			"vlan_mask": {
//				Type		:	schema.TypeInt,
//				Description	: 	"Vlan mask",
//				Optional	:	true,
//				Default		:	24,
//			},
//			"comment": {
//				Type		:	schema.TypeString,
//				Description	:	"Server name",
//				Required	:	true,
//			},
//			"status_code": {
//				Type		:	schema.TypeInt,
//				Description	:	"2- free, 1- assigned",
//				Optional	:	true,
//				Default		:	1,
//			},
//			"ip_address": {
//				Type		:	schema.TypeString,
//				Description	:	"Ip address if static",
//				Optional	:	true,
//				Computed	:	true,
//			},
//			"avoid_dhcp_scope": {
//				Type		:	schema.TypeBool,
//				Description	:	"If true will not set ip address in vlan with DHCP scope",
//				Optional	:	true,
//				Default		:	true,
//			},

//		},
//	}
//}

//func resourceIpCreate(d *schema.ResourceData, meta interface{}) error {
//	client := meta.(*gosolar.Client)
//	//declare vars
//	vlan_address := d.Get("vlan_address").(string)
//	comment := d.Get("comment").(string)
//	avoid_dhcp_scope := d.Get("avoid_dhcp_scope").(bool)
//	ip_address := d.Get("ip_address").(string)
//	status_code := d.Get("status_code").(int)
//	vlan_name := d.Get("vlan_name").(string)
//	vlan_mask := d.Get("vlan_mask").(int)

//	computedVlanName, VlanNameErr := getVlanName(client, vlan_address)
//	if VlanNameErr != nil {
//		return VlanNameErr
//	}
//	if vlan_name != "" && vlan_name != computedVlanName {
//		vlanNameMismatch := errors.New("There is mismatch in vlan name that you've provided (" + vlan_name + ") and computed value which is " + computedVlanName)
//		return vlanNameMismatch
//	} else if vlan_name == "" {
//		d.Set("vlan_name", computedVlanName)
//	}

//	subnetId, getSubnetErr := getSubnetId(client, vlan_address)
//	if getSubnetErr != nil {
//		return getSubnetErr
//	}
//	if avoid_dhcp_scope {
//		subnetDHCP,getSubnetDhcpErr := checkIfSubnetDHCP(client, vlan_address)
//		if getSubnetDhcpErr != nil {
//			return getSubnetDhcpErr
//		}
//		if subnetDHCP && ip_address == ""{
//			d.Set("ip_address", "dhcp")
//			d.SetId("dhcp")
//			return nil
//		} else if subnetDHCP && ip_address != "" {
//			subnetMismathErr := errors.New("You are trying to get static IP from subnet with DHCP Scope!")
//			return subnetMismathErr
//		}
//	}
//	if ip_address == "" {
//		ipEntity,getIpError := getFreeIpEntity(client, subnetId)
//		if getIpError != nil {
//			return getIpError
//		}
//		updateErr := updateIpEntity(client, *ipEntity, status_code, comment)
//		if updateErr != nil {
//			return updateErr
//		}
//		d.Set("ip_address", ipEntity.IPAddress)
//		d.SetId(ipEntity.IPAddress)
//		return nil
//	} else {
//		ipError := validateAddresses(ip_address)
//		if ipError != nil {
//			return ipError
//		}
//		ipSubnetError := validateAddresInSubnet(vlan_address, vlan_mask, ip_address)
//		if ipSubnetError != nil {
//			return ipSubnetError
//		}
//		ipEntity,getIpError := getIpEntityByAddress(client, ip_address)
//		if getIpError != nil {
//			return getIpError
//		}
//		if ipEntity.Status != 2 {
//			statusErr := errors.New("You are trying to get IP that is already assigned!")
//			return statusErr
//		}
//		updateErr := updateIpEntity(client, *ipEntity, status_code, comment)
//		if updateErr != nil {
//			return updateErr
//		}

//		d.Set("ip_address", ipEntity.IPAddress)
//		d.SetId(ipEntity.IPAddress)

//		return nil
//	}
//}

//func resourceIpRead(d *schema.ResourceData, meta interface{}) error {
//	client := meta.(*gosolar.Client)

//	//declare vars
//	id := d.Id()
//	vlan_address := d.Get("vlan_address").(string)
//	comment := d.Get("comment").(string)
//	avoid_dhcp_scope := d.Get("avoid_dhcp_scope").(bool)
//	ip_address := d.Get("ip_address").(string)
//	status_code := d.Get("status_code").(int)
//	vlan_name := d.Get("vlan_name").(string)

//	//Validate if it's dhcp error to handle
//	if id == "dhcp" && ip_address == "dhcp" {
//		return nil
//	}

//	computedVlanName, VlanNameErr := getVlanName(client, vlan_address)
//	if VlanNameErr != nil {
//		return VlanNameErr
//	}
//	if vlan_name != "" && vlan_name != computedVlanName {
//		vlanNameMismatch := errors.New("There is mismatch in vlan name that you've provided (" + vlan_name + ") and computed value which is " + computedVlanName)
//		return vlanNameMismatch
//	}

//	ipEntity,_ := getIpEntityByAddress(client, ip_address)

//	//Validate if provided ip address is assigned to this machine
//	if !strings.Contains(ipEntity.Comments, comment) && ipEntity.Status != 2 {
//		assignError := errors.New("IP address " + ip_address + " is not assigned to " + comment)
//		return assignError
//	}

//	//Validate if provided subnet is DHCP
//	dhcpScope,dhcpErr := checkIfSubnetDHCP(client, vlan_address)
//	if dhcpErr != nil {
//		return  dhcpErr
//	}
//	//Validate if subnet is DHCP AND avoid_dhcp_scope is true
//	if avoid_dhcp_scope && dhcpScope {
//		dhcpError := errors.New("avoid_dhcp_flag set to true, but subnet HAS dhcp scope")
//		return dhcpError
//	}

//	d.Set("ip_address", ipEntity.IPAddress)

//	return nil
//}

//func resourceIpUpdate(d *schema.ResourceData, meta interface{}) error {
//	client := meta.(*gosolar.Client)

//	//declare vars
//	comment := d.Get("comment").(string)
//	vlan_address := d.Get("vlan_address").(string)
//	vlan_name := d.Get("vlan_name").(string)

//	computedVlanName, VlanNameErr := getVlanName(client, vlan_address)
//	if VlanNameErr != nil {
//		return VlanNameErr
//	}
//	if vlan_name != "" && vlan_name != computedVlanName {
//		d.Set("vlan_name", computedVlanName)
//		vlanNameMismatch := errors.New("There is mismatch in vlan name that you've provided (" + vlan_name + ") and computed value which is " + computedVlanName)
//		return vlanNameMismatch
//	}
//	ip_address := d.Id()
//	if ip_address == "dhcp" {
//		return nil
//	}
//	status_code := d.Get("status_code").(int)

//	ipEntity,_ := getIpEntityByAddress(client, ip_address)
//	updateIpEntity(client, *ipEntity, status_code, comment)

//	d.Set("ip_address", ip_address)

//	return nil
//}

//func resourceIpDelete(d *schema.ResourceData, meta interface{}) error {
//	client := meta.(*gosolar.Client)
//	if d.Id() == "dhcp" {
//		return nil
//	}
//	ip_address := d.Get("ip_address").(string)
//	ipEntity,_ := getIpEntityByAddress(client, ip_address)

//	updateIpEntity(client, *ipEntity, 2, "")
//	return nil
//}

//func resourceIpImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
//	client := meta.(*gosolar.Client)
//	ip_address := d.Id()
//	ipEntity, ipEntityErr :=getIpEntityByAddress(client, ip_address)
//	if ipEntityErr != nil {
//		return nil, ipEntityErr
//	}
//	vlanAddress, vlanAddressErr := getSubnetAddress(client, ipEntity.SubnetId)
//	if vlanAddressErr != nil {
//		return nil, vlanAddressErr
//	}
//	computedVlanName, VlanNameErr := getVlanName(client, vlanAddress)
//	if VlanNameErr != nil {
//		return nil, VlanNameErr
//	}
//	d.Set("comment", ipEntity.Comments)
//	d.Set("status_code", ipEntity.Status)
//	d.Set("vlan_address", vlanAddress)
//	d.Set("ip_address", ip_address)
//	d.Set("vlan_name", computedVlanName)

//	return []*schema.ResourceData{d}, nil
//}
