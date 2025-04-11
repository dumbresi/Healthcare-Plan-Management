package elastic

import (
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

type Client struct {
	ES *elasticsearch.Client
}

type Factory struct{}

func NewElasticFactory() *Factory {
	return &Factory{}
}

func (f *Factory) NewClient(cfg elasticsearch.Config) (*Client, error) {
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Printf("Error creating the elasticsearch client: %s", err)
		return nil, err
	}

	return &Client{ES: es}, nil
}