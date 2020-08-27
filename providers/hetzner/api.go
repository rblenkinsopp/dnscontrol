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
	"time"
)

const ApiEndpoint = "https://dns.hetzner.com/api/v1"

// The Hetzner API uses a weird format for the timestamp which we have to deal with specially
type Timestamp struct {
	time.Time
}

func (c *Timestamp) UnmarshalJSON(data []byte) error {
	input, err := strconv.Unquote(string(data))

	if input == "" {
		c.Time = time.Time{}
		return nil
	}

	c.Time, err = time.Parse("2006-01-02 15:04:05.999 -0700 MST", input)
	return err
}

type Zone struct {
	Id              string    `json:"id,omitempty"`
	Created         Timestamp `json:"created,omitempty"`
	Modified        Timestamp `json:"modified,omitempty"`
	LegacyDNSHost   string    `json:"legacy_dns_host,omitempty"`
	LegacyNS        []string  `json:"legacy_ns,omitempty"`
	Name            string    `json:"name"`
	NS              []string  `json:"ns,omitempty"`
	Owner           string    `json:"owner,omitempty"`
	Paused          bool      `json:"paused,omitempty"`
	Permission      string    `json:"permission,omitempty"`
	Project         string    `json:"project,omitempty"`
	Registrar       string    `json:"registrar,omitempty"`
	Status          string    `json:"status,omitempty"`
	TTL             uint64    `json:"ttl"`
	Verified        Timestamp `json:"verified,omitempty"`
	RecordsCount    uint64    `json:"records_count,omitempty"`
	IsSecondaryDNS  bool      `json:"is_secondary_dns,omitempty"`
	TXTVerification struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	} `json:"txt_verification,omitempty"`
}

type Record struct {
	Type     string    `json:"type"`
	Id       string    `json:"id,omitempty"`
	Created  Timestamp `json:"created,omitempty"`
	Modified Timestamp `json:"modified,omitempty"`
	ZoneId   string    `json:"zone_id"`
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	TTL      uint64    `json:"ttl"`
}

type Meta struct {
	Pagination Pagination `json:"pagination,omitempty"`
}
//"meta":{"pagination":{"page":1,"per_page":2,"previous_page":1,"next_page":2,"last_page":2,"total_entries":3}}}

type Pagination struct {
	Page         uint64 `json:"page"`
	PerPage      uint64 `json:"per_page"`
	LastPage     uint64 `json:"last_page"`
	TotalEntries uint64 `json:"total_entries"`
}

type HdnsApiClient struct {
	apiToken   string
	httpClient *http.Client
}

func NewHdnsApiClient(apiToken string) *HdnsApiClient {
	return &HdnsApiClient{
		apiToken: apiToken,
		httpClient: &http.Client{
			Transport: nil,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Jar:     nil,
			Timeout: 0,
		},
	}
}

func (c *HdnsApiClient) request(method string, path string, queryStrings url.Values, input interface{}, output interface{}) (*Meta, error) {
	apiUrl, err := url.Parse(ApiEndpoint)
	if err != nil {
		return nil, err
	}
	apiUrl.Path += path

	if queryStrings != nil {
		apiUrl.RawQuery = queryStrings.Encode()
	}

	var body io.Reader = nil
	if input != nil {
		j, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(j)
	}

	request, err := http.NewRequest(method, apiUrl.String(), body)
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

func (c *HdnsApiClient) GetZones(name string) ([]Zone, error) {
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

func (c *HdnsApiClient) GetZone(zoneId string) (Zone, error) {
	var response struct {
		Zone Zone `json:"zone"`
	}
	_, err := c.request(http.MethodGet, "/zones/"+zoneId, nil, nil, &response)
	return response.Zone, err
}

func (c *HdnsApiClient) CreateZone(zone Zone) (Zone, error) {
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

func (c *HdnsApiClient) UpdateZone(zone Zone) (Zone, error) {
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

func (c *HdnsApiClient) DeleteZone(zone Zone) error {
	_, err := c.request(http.MethodDelete, "/zones/"+zone.Id, nil, nil, nil)
	return err
}

func (c *HdnsApiClient) GetRecords(zoneId string) ([]Record, error) {
	var response struct {
		Records []Record `json:"records"`
	}
	const method = http.MethodGet
	const path = "/records"
	parameters := url.Values{"zone_id": {zoneId}}

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

func (c *HdnsApiClient) GetRecord(recordId string) (Record, error) {
	var response struct {
		Record Record `json:"record"`
	}
	_, err := c.request(http.MethodGet, "/record/"+recordId, nil, nil, &response)
	return response.Record, err
}

func (c *HdnsApiClient) CreateRecord(record Record) (Record, error) {
	var response struct {
		Record Record `json:"record"`
	}
	_, err := c.request(http.MethodPost, "/records", nil, &record, &response)
	return response.Record, err
}

func (c *HdnsApiClient) UpdateRecord(record Record) (Record, error) {
	var response struct {
		Record Record `json:"record"`
	}
	_, err := c.request(http.MethodPut, "/records/"+record.Id, nil, &record, &response)
	return response.Record, err
}

func (c *HdnsApiClient) DeleteRecord(record Record) error {
	_, err := c.request(http.MethodDelete, "/records/"+record.Id, nil, nil, nil)
	return err
}
