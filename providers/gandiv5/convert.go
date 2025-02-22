package gandiv5

// Convert the provider's native record description to models.RecordConfig.

import (
	"fmt"

	"github.com/StackExchange/dnscontrol/v4/models"
	"github.com/StackExchange/dnscontrol/v4/pkg/printer"
	"github.com/StackExchange/dnscontrol/v4/pkg/txtutil"
	"github.com/go-gandi/go-gandi/livedns"
)

// nativeToRecord takes a DNS record from Gandi and returns a native RecordConfig struct.
func nativeToRecords(n livedns.DomainRecord, origin string) (rcs []*models.RecordConfig, err error) {
	// Gandi returns all the values for a given label/rtype pair in each
	// livedns.DomainRecord.  In other words, if there are multiple A
	// records for a label, all the IP addresses are listed in
	// n.RrsetValues rather than having many livedns.DomainRecord's.
	// We must split them out into individual records, one for each value.
	for _, value := range n.RrsetValues {
		rc := &models.RecordConfig{
			TTL:      uint32(n.RrsetTTL),
			Original: n,
		}
		rc.SetLabel(n.RrsetName, origin)

		switch rtype := n.RrsetType; rtype {
		case "ALIAS":
			rc.Type = "ALIAS"
			err = rc.SetTarget(value)
		default:
			err = rc.PopulateFromStringFunc(rtype, value, origin, txtutil.ParseQuoted)
		}
		if err != nil {
			return nil, fmt.Errorf("unparsable record received from gandi: %w", err)
		}

		rcs = append(rcs, rc)
	}

	return rcs, nil
}

func recordsToNative(rcs []*models.RecordConfig, origin string) []livedns.DomainRecord {
	// Take a list of RecordConfig and return an equivalent list of ZoneRecords.
	// Gandi requires one ZoneRecord for each label:key tuple, therefore we
	// might collapse many RecordConfig into one ZoneRecord.

	keys := map[models.RecordKey]*livedns.DomainRecord{}
	var zrs []livedns.DomainRecord

	for _, r := range rcs {
		label := r.GetLabel()
		if label == "@" {
			label = origin
		}
		key := r.Key()

		if zr, ok := keys[key]; !ok {
			// Allocate a new ZoneRecord:
			zr := livedns.DomainRecord{
				RrsetType:   r.Type,
				RrsetTTL:    int(r.TTL),
				RrsetName:   label,
				RrsetValues: []string{r.GetTargetCombinedFunc(txtutil.EncodeQuoted)},
			}
			keys[key] = &zr
		} else {
			zr.RrsetValues = append(zr.RrsetValues, r.GetTargetCombinedFunc(txtutil.EncodeQuoted))

			if r.TTL != uint32(zr.RrsetTTL) {
				printer.Warnf("All TTLs for a rrset (%v) must be the same. Using smaller of %v and %v.\n", key, r.TTL, zr.RrsetTTL)
				if r.TTL < uint32(zr.RrsetTTL) {
					zr.RrsetTTL = int(r.TTL)
				}
			}
		}
	}

	for _, zr := range keys {
		zrs = append(zrs, *zr)
	}
	return zrs
}
