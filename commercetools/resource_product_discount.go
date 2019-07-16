package commercetools

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/labd/commercetools-go-sdk/commercetools"
)

func resourceProductDiscount() *schema.Resource {
	return &schema.Resource{
		Create: resourceProductDiscountCreate,
		Read:   resourceProductDiscountRead,
		Update: resourceProductDiscountUpdate,
		Delete: resourceProductDiscountDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     TypeLocalizedString,
				Required: true,
			},
			"key": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"description": {
				Type:     TypeLocalizedString,
				Optional: true,
			},
			"predicate": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "1=1",
			},
			"sort_order": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"is_active": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"valid_from": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"valid_until": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"value": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validateProductDiscountType,
						},
						// Absolute specific fields
						"money": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"cent_amount": {
										Type:     schema.TypeInt,
										Required: true,
									},
									"currency_code": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: ValidateCurrencyCode,
									},
								},
							},
						},
						// Relative specific fields
						"permyriad": {
							Type:     schema.TypeInt,
							Optional: true,
						},
					},
				},
			},
			"version": {
				Type:     schema.TypeInt,
				Computed: true,
			},
		},
	}
}

func validateProductDiscountType(val interface{}, key string) (warns []string, errs []error) {
	var v = val.(string)

	switch v {
	case
		"external",
		"relative",
		"absolute":
		return
	default:
		errs = append(errs, fmt.Errorf("%q not a valid value for %q", val, key))
	}
	return
}

func resourceProductDiscountCreate(d *schema.ResourceData, m interface{}) error {
	client := getClient(m)

	name := expandLocalizedString(d.Get("name"))
	description := expandLocalizedString(d.Get("description"))

	draft := &commercetools.ProductDiscountDraft{
		Name:        &name,
		Key:         d.Get("key").(string),
		Description: &description,
		Predicate:   d.Get("predicate").(string),
		Value:       expandProductDiscountValue(d),
		SortOrder:   d.Get("sort_order").(string),
		IsActive:    d.Get("is_active").(bool),
	}

	if val := d.Get("valid_from").(string); len(val) > 0 {
		validFrom, err := expandDate(val)
		if err != nil {
			return err
		}
		draft.ValidFrom = &validFrom
	}
	if val := d.Get("valid_until").(string); len(val) > 0 {
		validUntil, err := expandDate(val)
		if err != nil {
			return err
		}
		draft.ValidUntil = &validUntil
	}

	log.Printf("[DEBUG] Going to create draft: %#v", draft)

	productDiscount, err := client.ProductDiscountCreate(draft)
	if err != nil {
		return err
	}

	d.SetId(productDiscount.ID)
	d.Set("version", productDiscount.Version)

	return resourceProductDiscountRead(d, m)
}

func expandLocalizedString(value interface{}) commercetools.LocalizedString {
	return commercetools.LocalizedString(
		expandStringMap(value.(map[string]interface{})))
}

func expandProductDiscountValue(d *schema.ResourceData) commercetools.ProductDiscountValue {
	value := d.Get("value").([]interface{})[0].(map[string]interface{})

	log.Printf("[DEBUG] Product discount value: %#v", value)

	switch value["type"].(string) {
	case "external":
		return commercetools.ProductDiscountValueExternal{}
	case "absolute":
		moneyData := value["money"].([]interface{})
		moneyList := make([]commercetools.Money, 0)
		for _, data := range moneyData {
			mapData := data.(map[string]interface{})
			currencyCode := mapData["currency_code"].(string)
			centAmount := mapData["cent_amount"].(int)
			money := commercetools.Money{
				CurrencyCode: commercetools.CurrencyCode(currencyCode),
				CentAmount:   centAmount,
			}
			moneyList = append(moneyList, money)
		}

		return commercetools.ProductDiscountValueAbsolute{
			Money: moneyList,
		}
	case "relative":
		return commercetools.ProductDiscountValueRelative{
			Permyriad: value["permyriad"].(int),
		}
	default:
		return fmt.Errorf("Unknown product discount type %s", value["type"])
	}
}

func flattenProductDiscountValue(productDiscount commercetools.ProductDiscountValue) (out map[string]interface{}) {
	log.Printf("[DEBUG] Trying to flatten %#v", productDiscount)
	out = make(map[string]interface{})
	if discount, ok := productDiscount.(commercetools.ProductDiscountValueAbsolute); ok {
		out["type"] = "absolute"
		out["money"] = flattenProductDiscountAbsolute(discount.Money)
		return out
	} else if discount, ok := productDiscount.(commercetools.ProductDiscountValueRelative); ok {
		out["type"] = "relative"
		out["permyriad"] = discount.Permyriad
		return out
	} else if _, ok := productDiscount.(commercetools.ProductDiscountValueExternal); ok {
		out["type"] = "external"
		return out
	}

	panic(fmt.Errorf("Failed to flatten product discount value"))
}

func flattenProductDiscountAbsolute(money []commercetools.Money) []map[string]interface{} {
	var out = make([]map[string]interface{}, len(money), len(money))
	for _, moneyEntry := range money {
		m := make(map[string]interface{})
		m["currency_code"] = string(moneyEntry.CurrencyCode)
		m["cent_amount"] = moneyEntry.CentAmount
		out = append(out, m)
	}
	return out
}

func resourceProductDiscountRead(d *schema.ResourceData, m interface{}) error {
	log.Print("[DEBUG] Reading product discount from commercetools")
	client := getClient(m)

	productDiscount, err := client.ProductDiscountGetWithID(d.Id())

	if err != nil {
		if ctErr, ok := err.(commercetools.ErrorResponse); ok {
			if ctErr.StatusCode == 404 {
				d.SetId("")
				return nil
			}
		}
		return err
	}

	if productDiscount == nil {
		log.Print("[DEBUG] No product type found")
		d.SetId("")
	} else {
		log.Printf("[DEBUG] Found following product discount: %#v", productDiscount)
		log.Print(stringFormatObject(productDiscount))

		d.Set("version", productDiscount.Version)
		d.Set("name", productDiscount.Name)
		d.Set("key", productDiscount.Key)
		d.Set("description", productDiscount.Description)
		if err := d.Set("value", []interface{}{flattenProductDiscountValue(productDiscount.Value)}); err != nil {
			return err
		}
		d.Set("predicate", productDiscount.Predicate)
		d.Set("sort_order", productDiscount.SortOrder)
		d.Set("is_active", productDiscount.IsActive)
		d.Set("valid_from", nil)
		if productDiscount.ValidFrom != nil {
			d.Set("valid_from", flattenDateToString(*productDiscount.ValidFrom))
		}
		d.Set("valid_until", nil)
		if productDiscount.ValidUntil != nil {
			d.Set("valid_until", flattenDateToString(*productDiscount.ValidUntil))
		}
	}

	return nil
}

func expandDate(input string) (time.Time, error) {
	return time.Parse("2006-01-02", input)
}

func flattenDateToString(input time.Time) string {
	return input.Format("2006-01-02")
}

func resourceProductDiscountUpdate(d *schema.ResourceData, m interface{}) error {
	client := getClient(m)

	input := &commercetools.ProductDiscountUpdateWithIDInput{
		ID:      d.Id(),
		Version: d.Get("version").(int),
		Actions: []commercetools.ProductDiscountUpdateAction{},
	}

	if d.HasChange("key") {
		newKey := d.Get("key").(string)
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountSetKeyAction{Key: newKey})
	}

	if d.HasChange("is_active") {
		isActive := d.Get("is_active").(bool)
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountChangeIsActiveAction{IsActive: isActive})
	}

	if d.HasChange("predicate") {
		newPredicate := d.Get("predicate").(string)
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountChangePredicateAction{Predicate: newPredicate})
	}

	if d.HasChange("sort_order") {
		newSortOrder := d.Get("sort_order").(string)
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountChangeSortOrderAction{SortOrder: newSortOrder})
	}

	if d.HasChange("valid_from") {
		validFrom, err := expandDate(d.Get("valid_from").(string))
		if err != nil {
			return err
		}
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountSetValidFromAction{ValidFrom: &validFrom})
	}

	if d.HasChange("valid_until") {
		validUntil, err := expandDate(d.Get("valid_until").(string))
		if err != nil {
			return err
		}
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountSetValidUntilAction{ValidUntil: &validUntil})
	}

	if d.HasChange("name") {
		newName := expandLocalizedString(d.Get("name"))
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountChangeNameAction{Name: &newName})
	}

	if d.HasChange("description") {
		newDescr := expandLocalizedString(d.Get("description"))
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountSetDescriptionAction{Description: &newDescr})
	}

	if d.HasChange("value") {
		newValue := expandProductDiscountValue(d)
		input.Actions = append(
			input.Actions,
			&commercetools.ProductDiscountChangeValueAction{Value: newValue})
	}


	log.Printf(
		"[DEBUG] Will perform update operation with the following actions:\n%s",
		stringFormatActions(input.Actions))

	_, err := client.ProductDiscountUpdateWithID(input)
	if err != nil {
		if ctErr, ok := err.(commercetools.ErrorResponse); ok {
			log.Printf("[DEBUG] %v: %v", ctErr, stringFormatErrorExtras(ctErr))
		}
		return err
	}

	return resourceProductDiscountRead(d, m)
}

func resourceProductDiscountDelete(d *schema.ResourceData, m interface{}) error {
	client := getClient(m)
	version := d.Get("version").(int)
	_, err := client.ProductDiscountDeleteWithID(d.Id(), version)
	if err != nil {
		return err
	}

	return nil
}