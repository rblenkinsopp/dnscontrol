package hetzner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
)

const apiEndpoint = "https://dns.hetzner.com/api/v1"

// Zone is minimal representation of a Hetzner zone
type Zone struct {
	ID   string   `json:"id,omitempty"`
	Name string   `json:"name"`
	NS   []string `json:"ns,omitempty"`
	TTL  uint64   `json:"ttl"`
}

// Record is a minimal representation of a Hetzner record
type Record struct {
	Type   string `json:"type"`
	ID     string `json:"id,omitempty"`
	ZoneID string `json:"zone_id"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	TTL    uint64 `json:"ttl"`
}

// Meta contains extra metadata on a result
type Meta struct {
	Pagination Pagination `json:"pagination,omitempty"`
}

// Pagination lists the current and last page
type Pagination struct {
	Page         uint64 `json:"page"`
	LastPage     uint64 `json:"last_page"`
}

// APIClient is a Hetzner API client
type APIClient struct {
	apiToken   string
	httpClient *http.Client
}

// NewAPIClient creates a new Hetzner API Client
func NewAPIClient(apiToken string) *APIClient {
	return &APIClient{
		apiToken: apiToken,
		httpClient: &http.Client{},
	}
}

func (c *APIClient) request(method string, path string, queryStrings url.Values, input interface{}, output interface{}) (*Meta, error) {
	apiURL, err := url.Parse(apiEndpoint)
	if err != nil {
		return nil, err
	}
	apiURL.Path += path

	if queryStrings != nil {
		apiURL.RawQuery = queryStrings.Encode()
	}

	var body io.Reader = nil
	if input != nil {
		j, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(j)
	}

	request, err := http.NewRequest(method, apiURL.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Auth-API-Token", c.apiToken)
	//request.Header.Add("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		// Attempt to get the error message
		var errorResponse struct {
			Message string `json:"message"`
		}

		content, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(content, &errorResponse); err != nil {
			return nil, fmt.Errorf(string(content))
		}

		return nil, fmt.Errorf(errorResponse.Message)
	}

	jsonData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if output != nil {
		if err := json.Unmarshal(jsonData, &output); err != nil {
			return nil, err
		}
	}

	var meta struct {
		Meta Meta `json:"meta"`
	}
	if err := json.Unmarshal(jsonData, &meta); err == nil {
		return &meta.Meta, nil
	}

	return nil, nil
}

// GetZones returns all zones associated with the account
func (c *APIClient) GetZones(name string) ([]Zone, error) {
	var response struct {
		Zones []Zone `json:"zones"`
	}
	const method = http.MethodGet
	const path = "/zones"
	parameters := url.Values{"name": {name}, "per_page": {"2"}}

	meta, err := c.request(method, path, parameters, nil, &response)
	for meta != nil && meta.Pagination.Page < meta.Pagination.LastPage {
		parameters.Set("page", strconv.FormatUint(meta.Pagination.Page+1, 10))

		extraResponse := response
		meta, err = c.request(method, path, parameters, nil, &extraResponse)
		if err != nil {
			return nil, err
		}

		for _, zone := range extraResponse.Zones {
			extraResponse.Zones = append(extraResponse.Zones, zone)
		}
	}

	return response.Zones, err
}

// GetZone returns an Zone object represented by the zoneID
func (c *APIClient) GetZone(zoneID string) (Zone, error) {
	var response struct {
		Zone Zone `json:"zone"`
	}
	_, err := c.request(http.MethodGet, "/zones/"+zoneID, nil, nil, &response)
	return response.Zone, err
}

// CreateZone creates a new zone
func (c *APIClient) CreateZone(zone Zone) (Zone, error) {
	request := struct {
		Name string `json:"name"`
		TTL  uint64 `json:"ttl"`
	}{
		Name: zone.Name,
		TTL:  zone.TTL,
	}
	var response struct {
		Zone Zone `json:"zone"`
	}
	_, err := c.request(http.MethodPost, "/zones", nil, &request, &response)
	return response.Zone, err
}

// UpdateZone updates an existing zone
func (c *APIClient) UpdateZone(zone Zone) (Zone, error) {
	request := struct {
		Name string `json:"name"`
		TTL  uint64 `json:"ttl"`
	}{
		Name: zone.Name,
		TTL:  zone.TTL,
	}
	var response struct {
		Zone Zone `json:"zone"`
	}
	_, err := c.request(http.MethodPut, "/zones", nil, &request, &response)
	return response.Zone, err
}

// DeleteZone deletes and existing zone
func (c *APIClient) DeleteZone(zone Zone) error {
	_, err := c.request(http.MethodDelete, "/zones/"+zone.ID, nil, nil, nil)
	return err
}

// GetRecords returns all records associated with the zoneID
func (c *APIClient) GetRecords(zoneID string) ([]Record, error) {
	var response struct {
		Records []Record `json:"records"`
	}
	const method = http.MethodGet
	const path = "/records"
	parameters := url.Values{"zone_id": {zoneID}}

	meta, err := c.request(method, path, parameters, nil, &response)
	for meta != nil && meta.Pagination.Page < meta.Pagination.LastPage {
		parameters.Set("page", strconv.FormatUint(meta.Pagination.Page+1, 10))

		extraResponse := response
		meta, err = c.request(method, path, parameters, nil, &extraResponse)
		if err != nil {
			return nil, err
		}

		for _, zone := range extraResponse.Records {
			extraResponse.Records = append(extraResponse.Records, zone)
		}
	}

	return response.Records, err
}

// GetRecord returns a existing record based on the recordID
func (c *APIClient) GetRecord(recordID string) (Record, error) {
	var response struct {
		Record Record `json:"record"`
	}
	_, err := c.request(http.MethodGet, "/record/"+recordID, nil, nil, &response)
	return response.Record, err
}

// CreateRecord creates a new record
func (c *APIClient) CreateRecord(record Record) (Record, error) {
	var response struct {
		Record Record `json:"record"`
	}
	_, err := c.request(http.MethodPost, "/records", nil, &record, &response)
	return response.Record, err
}

// UpdateRecord updates and existing record
func (c *APIClient) UpdateRecord(record Record) (Record, error) {
	var response struct {
		Record Record `json:"record"`
	}
	_, err := c.request(http.MethodPut, "/records/"+record.ID, nil, &record, &response)
	return response.Record, err
}

// DeleteRecord deletes and existing record
func (c *APIClient) DeleteRecord(record Record) error {
	_, err := c.request(http.MethodDelete, "/records/"+record.ID, nil, nil, nil)
	return err
}
