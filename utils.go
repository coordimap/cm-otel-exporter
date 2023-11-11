package cmotelexporter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	cmotel "github.com/coordimap/cm-otel-go"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func encodeAndHashElement(postgresElem interface{}) ([]byte, string, error) {
	marshaled, errMarshaled := json.Marshal(postgresElem)
	if errMarshaled != nil {
		return []byte{}, "", errMarshaled
	}

	hashArr := sha256.Sum256(marshaled)
	hashStr := hex.EncodeToString(hashArr[:])

	return marshaled, hashStr, nil
}

// CreateElement create a generic element
func CreateElement(element interface{}, name, id, elemType string, crawlTime time.Time) (*Element, error) {
	marshaled, hashed, err := encodeAndHashElement(element)
	if err != nil {
		return nil, err
	}

	return &Element{
		RetrievedAt: crawlTime,
		Name:        name,
		ID:          id,
		Type:        elemType,
		Hash:        hashed,
		Data:        marshaled,
		IsJSONData:  true,
	}, nil
}

// CreateRelationship create a relationship element
func CreateRelationship(sourceID, destinationID, relationshipType, wrapperRelationshipType string, crawlTime time.Time) (*Element, error) {
	parentElem := RelationshipElement{
		SourceID:         sourceID,
		DestinationID:    destinationID,
		RelationshipType: relationshipType,
	}

	relationshipWrapperElem, errRelationshipWrapperElem := CreateElement(
		parentElem,
		fmt.Sprintf("%s.%s", parentElem.SourceID, parentElem.DestinationID),
		fmt.Sprintf("%s.%s", parentElem.SourceID, parentElem.DestinationID),
		wrapperRelationshipType,
		crawlTime,
	)
	if errRelationshipWrapperElem != nil {
		return nil, errRelationshipWrapperElem
	}

	return relationshipWrapperElem, nil
}

func relationshipExists(from string, to string, existingRelationshipIds map[string]string) bool {
	keyName := fmt.Sprintf("%s.%s", from, to)

	return keyExists(keyName, existingRelationshipIds)
}

func keyExists(key string, mapToCheck map[string]string) bool {
	_, exists := mapToCheck[key]

	return exists
}

func shouldCreateNewRelationship(from string, to string, mapToCheck map[string]string) bool {
	return keyExists(from, mapToCheck) &&
		keyExists(to, mapToCheck) &&
		relationshipExists(from, to, mapToCheck)
}

func getRelationshipsFromResourceAttributes(attrs []attribute.KeyValue, existingRelationshipIDs map[string]string) []*Element {
	newFoundRelationships := []*Element{}
	relationshipKeysToAdd := []exporterResourceRelationshipKeys{
		{
			From: cmotel.EnvK8SClusterName,
			To:   string(semconv.ServiceNameKey),
		},
		{
			From: cmotel.EnvK8SClusterName,
			To:   cmotel.EnvServiceAccountType,
		},
		{
			From: cmotel.EnvK8SClusterName,
			To:   cmotel.PodNameCompleteType,
		},
		{
			From: cmotel.EnvNodeNameType,
			To:   cmotel.EnvK8SClusterName,
		},
		{
			From: cmotel.EnvNodeNameType,
			To:   cmotel.PodNameCompleteType,
		},
		{
			From: cmotel.EnvNodeNameType,
			To:   string(semconv.ServiceNameKey),
		},
	}

	// add all the known resources in a map
	mappedVars := map[string]string{}

	for _, attr := range attrs {
		mappedVars[string(attr.Key)] = attr.Value.AsString()
	}

	// compile the relationships
	for _, relFromTo := range relationshipKeysToAdd {
		// check if from or to are empty strings then continue
		if relFromTo.From == "" || relFromTo.To == "" {
			continue
		}

		from, fromExists := mappedVars[relFromTo.From]
		to, toExists := mappedVars[relFromTo.To]

		// check if the keys exists in the map and they are not empty
		if !(fromExists && toExists) && (mappedVars[from] != "" && mappedVars[to] != "") {
			continue
		}

		if shouldCreateNewRelationship(mappedVars[from], mappedVars[to], existingRelationshipIDs) {
			newRel, errNewRel := CreateRelationship(mappedVars[from], mappedVars[to], cmotel.OtelComponentRelationship, cmotel.ComponentRelationshipSkipInsert, time.Now())
			if errNewRel != nil {
				// TODO: log sth here
			} else {
				existingRelationshipIDs[newRel.ID] = ""
				newFoundRelationships = append(newFoundRelationships, newRel)
			}
		}
	}

	return newFoundRelationships

}

func loadComponent() {

}
