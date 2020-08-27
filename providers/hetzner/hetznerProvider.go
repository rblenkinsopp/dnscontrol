package hetzner

import (
	"encoding/json"
	"fmt"

	"github.com/StackExchange/dnscontrol/v3/models"
	"github.com/StackExchange/dnscontrol/v3/pkg/diff"
	"github.com/StackExchange/dnscontrol/v3/providers"
)

/*
Hetzner DNS provider (dns.hetzner.com)

Info required in `creds.json`:
	- api-token

Supported record types:
    - A
    - AAAA
    - NS
    - MX
    - CNAME
    - RP
    - TXT 
    - SOA
    - HINFO
    - SRV
    - DANE
    - TLSA
    - DS
    - CAA

*/

var features = providers.DocumentationNotes{
	providers.CanUseAlias:            providers.Cannot(),
	providers.CanUseCAA:              providers.Can(),
	providers.CanUseNAPTR:            providers.Cannot(),
	providers.CanUseDS:               providers.Can(),
	providers.CanUseDSForChildren:    providers.Can(),
	providers.CanUsePTR:              providers.Cannot(),
	providers.CanUseSSHFP:            providers.Cannot(),
	providers.CanUseSRV:              providers.Can(),
	providers.CanUseTLSA:             providers.Can(),
	providers.CanUseTXTMulti:         providers.Can(),
	providers.CanAutoDNSSEC:          providers.Can(),
	providers.DocCreateDomains:       providers.Can(),
	providers.DocDualHost:            providers.Can(),
	providers.DocOfficiallySupported: providers.Can(),
	providers.CanGetZones:            providers.Can(),
}

func init() {
	providers.RegisterDomainServiceProviderType("HETZNER", NewProvider, features)
}

type HdnsProvider struct {
	client *HdnsApiClient
}

func NewProvider(cfg map[string]string, _ json.RawMessage) (providers.DNSServiceProvider, error) {
	apiToken := cfg["api-token"]

	if apiToken == "" {
		return nil, fmt.Errorf("api-token must be provided")
	}

	provider := &HdnsProvider{
		client: NewHdnsApiClient(apiToken),
	}

	return provider, nil
}

// ListZones list
func (c *HdnsProvider) ListZones() ([]string, error) {
	zones, err := c.client.GetZones("")
	if err != nil {
		return nil, err
	}

	var zoneNames []string
	for _, zone := range zones {
		zoneNames = append(zoneNames, zone.Name)
	}

	return zoneNames, nil
}

// EnsureDomainExists creates the domain if it does not exist.
func (c *HdnsProvider) EnsureDomainExists(domain string) error {
	zones, err := c.ListZones()
	if err != nil {
		return err
	}

	for _, zone := range zones {
		if zone == domain {
			return nil
		}
	}

	_, err = c.client.CreateZone(Zone{
		Name: "domain",
		TTL:  86400,
	})

	return err
}

func (c *HdnsProvider) GetNameservers(domain string) ([]*models.Nameserver, error) {
	zones, err := c.client.GetZones(domain)
	if err != nil {
		return nil, err
	}

	nameservers, err := models.ToNameservers(zones[0].NS)
	return nameservers, err
}

// GetDomainCorrections returns a list of corretions for the  domain.
func (c *HdnsProvider) GetDomainCorrections(dc *models.DomainConfig) ([]*models.Correction, error) {
	var corrections []*models.Correction

	err := dc.Punycode()
	if err != nil {
		return nil, err
	}

	records, err := c.GetZoneRecords(dc.Name)
	if err != nil {
		return nil, err
	}

	// Remove the SOA record as this is locked and cannot be altered
	var prunedRecords models.Records
	for _, r := range records {
		if r.Type != "SOA" {
			prunedRecords = append(prunedRecords, r)
		}
	}

	// Normalize
	models.PostProcessRecords(prunedRecords)

	differ := diff.New(dc)
	_, toCreate, toDelete, toModify, err := differ.IncrementalDiff(prunedRecords)
	if err != nil {
		return nil, err
	}

	for _, del := range toDelete {
		record := del.Existing.Original.(Record)
		corrections = append(corrections, &models.Correction{
			Msg: del.String(),
			F:   func() error { return c.client.DeleteRecord(record) },
		})
	}

	for _, cre := range toCreate {
		record := Record{
			Type:   cre.Desired.Type,
			ZoneId: dc.Name,
			Name:   cre.Desired.Name,
			Value:  cre.Desired.GetTargetCombined(),
			TTL:    uint64(cre.Desired.TTL),
		}
		corrections = append(corrections, &models.Correction{
			Msg: cre.String(),
			F: func() error {
				_, err := c.client.CreateRecord(record)
				return err
			},
		})
	}

	for _, mod := range toModify {
		record := Record{
			Type:   mod.Desired.Type,
			Id:     mod.Existing.Original.(Record).Id,
			ZoneId: dc.Name,
			Name:   mod.Desired.Name,
			Value:  mod.Desired.GetTargetCombined(),
			TTL:    uint64(mod.Desired.TTL),
		}
		corrections = append(corrections, &models.Correction{
			Msg: mod.String(),
			F: func() error {
				_, err := c.client.UpdateRecord(record)
				return err
			},
		})
	}

	return corrections, err
}

func (c *HdnsProvider) GetZoneRecords(domain string) (models.Records, error) {
	zones, err := c.client.GetZones(domain)
	if err != nil {
		return nil, err
	}

	zone := zones[0]

	records, err := c.client.GetRecords(zone.Id)
	if err != nil {
		return nil, err
	}

	var rcs []*models.RecordConfig
	for _, record := range records {
		rc := &models.RecordConfig{
			Type:     record.Type,
			Name:     record.Name,
			TTL:      uint32(record.TTL),
			Metadata: nil,
			Original: record,
		}
		rc.SetLabel(record.Name, domain)
		err := rc.PopulateFromString(record.Type, record.Value, zone.Name)
		if err != nil {
			return nil, err
		}
		rcs = append(rcs, rc)
	}

	return rcs, nil
}
