package cmotelexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	cmotel "github.com/coordimap/cm-otel-go"
	"go.opentelemetry.io/otel/sdk/trace"
)

const defaultCoordimapEndpointURL = "https://api.coordimap.com/collector/crawlers/otel"

// CoordimapExporter is a custom OpenTelemetry span exporter that sends spans to an HTTP endpoint.
type CoordimapExporter struct {
	EndpointURL      string
	dataSourceInfo   DataSourceInfo
	dataSourceConfig DataSourceConfig
	CoordimapAPIKey  string
}

var _ trace.SpanExporter = &CoordimapExporter{}

// NewCoordimapExporter creates a new instance of CustomExporter.
func NewCoordimapExporter(options ...func(*CoordimapExporter) error) (*CoordimapExporter, error) {
	exporter := &CoordimapExporter{
		EndpointURL:      defaultCoordimapEndpointURL,
		dataSourceConfig: DataSourceConfig{},
		dataSourceInfo: DataSourceInfo{
			Name: "SampleTraceExporterName",
			Desc: "Sample Trace Exporter Name",
			Type: "opentelemetry",
		},
		CoordimapAPIKey: "",
	}

	// Apply options
	for _, option := range options {
		if err := option(exporter); err != nil {
			return nil, err
		}
	}

	if exporter.CoordimapAPIKey == "" {
		return nil, errors.New("The Coordimap API has not been set")
	}

	return exporter, nil
}

// WithEndpoint sets the endpoint URL for the custom exporter.
func WithEndpoint(endpointURL string) func(*CoordimapExporter) error {
	return func(exporter *CoordimapExporter) error {
		if endpointURL == "" {
			return errors.New("endpointURL cannot be empty")
		}

		exporter.EndpointURL = endpointURL
		return nil
	}
}

// WithDataSourceInfoName sets the name of the data source.
func WithDataSourceInfoName(name string) func(*CoordimapExporter) error {
	return func(exporter *CoordimapExporter) error {
		if name == "" {
			return errors.New("Data Source Info name cannot be empty")
		}

		exporter.dataSourceInfo.Name = name
		return nil
	}
}

// WithDataSourceInfoDescription sets the description of the data source.
func WithDataSourceInfoDescription(description string) func(*CoordimapExporter) error {
	return func(exporter *CoordimapExporter) error {
		if description == "" {
			return errors.New("Data Source Info description cannot be empty")
		}

		exporter.dataSourceInfo.Desc = description
		return nil
	}
}

// WithCoordimapAPIKEy sets the coordimap API Key.
func WithCoordimapAPIKEy(apiKey string) func(*CoordimapExporter) error {
	return func(exporter *CoordimapExporter) error {
		if apiKey == "" {
			return errors.New("The Coordimap API Key cannot be empty")
		}

		exporter.CoordimapAPIKey = apiKey
		return nil
	}
}

// ExportSpans exports a batch of span data to the HTTP endpoint.
func (e *CoordimapExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	allElements := []*Element{}
	foundRelationshipIDs := map[string]string{}

	for _, span := range spans {
		/**
		1. create a map from the key/value pairs
		2. check if the map contains the relationships
		3. check if the map contains a component
			- get the type of the component
			- check if the map contains all the required fields
			- get all the optional fields
		*/

		attrMap := map[string]string{}
		for _, attr := range span.Attributes() {
			attrMap[string(attr.Key)] = attr.Value.AsString()
		}

		newRelationships := getRelationshipsFromResourceAttributes(span.Resource().Attributes(), foundRelationshipIDs)

		if len(newRelationships) > 0 {
			allElements = append(allElements, newRelationships...)
		}

		for _, link := range span.Links() {
			for _, linkAttr := range link.Attributes {
				if string(linkAttr.Key) == cmotel.SpanAttrRelationship {
					rels := strings.Split(linkAttr.Value.AsString(), "@@@")

					spanRelElem, errSpanRelElem := CreateRelationship(rels[0], rels[1], cmotel.OtelComponentRelationship, cmotel.ComponentRelationshipSkipInsert, span.EndTime())
					if errSpanRelElem != nil {
						// TODO: log sth here
						continue
					}

					allElements = append(allElements, spanRelElem)
					break
				}
			}
		}

		// get all relationships found in the span
		allElements = append(allElements, getSpanRelationships(attrMap, span.Name(), span.EndTime())...)

		// check for spancomponent
		if componentData, ok := attrMap[cmotel.SpanAttrComponent]; ok {
			var spanComponent cmotel.CMComponent
			errSpanComponent := json.Unmarshal([]byte(componentData), &spanComponent)
			if errSpanComponent != nil {
				// TODO: log here
				continue
			}

			elem, errElem := CreateElement(spanComponent, spanComponent.Name, spanComponent.InternalID, cmotel.ComponentType, span.EndTime())
			if errElem != nil {
				// TODO: log sth here
				continue
			}

			allElements = append(allElements, elem)
		}
	}

	fmt.Printf("All Elements: %v \n", allElements)

	crawledData := &CrawledData{
		Data: allElements,
	}

	dataToSend := &CloudCrawlData{
		CrawledData: *crawledData,
		DataSource: DataSource{
			Info: e.dataSourceInfo,
			Config: DataSourceConfig{
				ValuePairs: []KeyValue{},
			},
		},
		Timestamp:       time.Now(),
		CrawlInternalID: "otel-coordimap-exporter",
	}

	traceDataBytes, err := json.Marshal(dataToSend)
	if err != nil {
		return err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", e.EndpointURL, bytes.NewBuffer(traceDataBytes))
	if err != nil {
		fmt.Println(err)
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Api-Key", fmt.Sprintf("%s", e.CoordimapAPIKey))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status code: %d", resp.StatusCode)
	}

	return nil
}

// Shutdown performs any necessary cleanup when the exporter is shut down.
func (e *CoordimapExporter) Shutdown(ctx context.Context) error {
	// Perform any cleanup here if needed.
	return nil
}
