package queue

// Client is a convenience contract for implementations that expose both
// producer and consumer behavior against Cloudflare Queues.
type Client interface {
	Producer
	Consumer
}
