package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/juju/terraform-provider-juju/internal/juju"
)

func resourceAccessModel() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "A resource that represent a Juju Access Model.",

		CreateContext: resourceAccessModelCreate,
		ReadContext:   resourceAccessModelRead,
		UpdateContext: resourceAccessModelUpdate,
		DeleteContext: resourceAccessModelDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"model": {
				Description: "The name of the model for access management",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"users": {
				Description: "List of users to grant access to",
				Type:        schema.TypeList,
				Required:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"access": {
				Description: "Type of access to the model",
				Type:        schema.TypeString,
				Required:    true,
			},
		},
	}
}

func resourceAccessModelCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*juju.Client)

	var diags diag.Diagnostics

	model := d.Get("model").(string)
	access := d.Get("access").(string)
	users := d.Get("users").([]string)

	uuid, err := client.Models.ResolveModelUUID(model)
	if err != nil {
		return diag.FromErr(err)
	}

	modelUUIDs := []string{uuid}

	for _, user := range users {
		err := client.Models.GrantModel(juju.GrantModelInput{
			User:       user,
			Access:     access,
			ModelUUIDs: modelUUIDs,
		})
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(fmt.Sprintf("%s:%s", model, access))

	return diags
}

func resourceAccessModelRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*juju.Client)

	var diags diag.Diagnostics

	id := strings.Split(d.Id(), ":")

	uuid, err := client.Models.ResolveModelUUID(id[0])
	if err != nil {
		return diag.FromErr(err)
	}
	response, err := client.Users.ModelUserInfo(uuid)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("model", id[0]); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("access", id[1]); err != nil {
		return diag.FromErr(err)
	}

	users := []string{}

	for _, modelUser := range response.ModelUserInfo {
		if string(modelUser.Access) == id[1] {
			users = append(users, modelUser.UserName)
		}
	}

	if err = d.Set("users", users); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

// Updating the access model supports three cases
// access and users both changed:
// for missing users - revoke access
// for changed users - apply new access
// users changed:
// for missing users - revoke access
// for new users - apply access
// access changed - apply new access
func resourceAccessModelUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*juju.Client)

	var diags diag.Diagnostics
	anyChange := false

	// items that could be changed
	var newAccess string
	var newUsersList []string
	var missingUserList []string

	var err error

	if d.HasChange("users") {
		anyChange = true
		oldUsers, newUsers := d.GetChange("users")
		oldUsersList := oldUsers.([]string)
		newUsersList = newUsers.([]string)
		missingUserList = getMissingUsers(oldUsersList, newUsersList)
	}

	if d.HasChange("access") {
		anyChange = true
		_, accessChanged := d.GetChange("access")
		newAccess = accessChanged.(string)
	}

	if !anyChange {
		return diags
	}

	err = client.Models.UpdateAccessModel(juju.UpdateAccessModelInput{
		Model:  d.Id(),
		Grant:  newUsersList,
		Revoke: missingUserList,
		Access: newAccess,
	})
	if err != nil {
		return diag.FromErr(err)
	}

	if newAccess != "" {
		id := strings.Split(d.Id(), ":")
		model := id[0]
		d.SetId(fmt.Sprintf("%s:%s", model, newAccess))
	}

	return diags
}

func getMissingUsers(oldUsers, newUsers []string) []string {
	var missing []string
	for _, user := range oldUsers {
		found := false
		for _, newUser := range newUsers {
			if user == newUser {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, user)
		}
	}
	return missing
}

// Juju refers to deletions as "destroy" so we call the Destroy function of our client here rather than delete
// This function remains named Delete for parity across the provider and to stick within terraform naming conventions
func resourceAccessModelDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*juju.Client)

	var diags diag.Diagnostics

	users := d.Get("users").([]string)
	access := d.Get("access").(string)

	err := client.Models.DestroyAccessModel(juju.DestroyAccessModelInput{
		Model:  d.Id(),
		Revoke: users,
		Access: access,
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}
