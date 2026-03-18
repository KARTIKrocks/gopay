// Package gopay provides a unified interface for payment processing
// across multiple providers including Stripe, PayPal, and Razorpay.
//
// Each provider lives in its own sub-module so that importing one provider
// does not pull in the dependencies of another. For example, using PayPal
// will not require the Stripe SDK.
//
// # Core Package
//
// This package defines the shared interfaces ([Provider], [CustomerProvider],
// [PaymentMethodProvider], [WebhookProvider]), request/response types, sentinel
// errors, and the [Client] that wraps any provider with validation and
// convenience methods.
//
//	client, err := gopay.NewClient(provider)
//	p, err := client.CreatePayment(ctx, gopay.NewPaymentRequest(gopay.USD(1999)))
//
// # Providers
//
// Install only the providers you need:
//
//	go get github.com/KARTIKrocks/gopay/stripe
//	go get github.com/KARTIKrocks/gopay/paypal
//	go get github.com/KARTIKrocks/gopay/razorpay
//
// Each provider package exports a Config, a Provider (which implements
// [Provider] and optionally [CustomerProvider] / [PaymentMethodProvider] /
// [WebhookProvider]), and a NewProvider constructor.
//
// # Testing
//
// Use [MockProvider] for unit tests without hitting any external API:
//
//	mock := gopay.NewMockProvider()
//	mock.WithAutoSucceed(true)
//	client, err := gopay.NewClient(mock)
//
// # Webhooks
//
// Use [WebhookProvider].VerifyWebhook to verify and parse incoming webhook
// events. Each provider package also exports a ParseWebhook function that
// parses the payload without signature verification — this is useful for
// debugging or when verification is handled elsewhere (e.g. by a gateway),
// but should not be used in production without separate verification.
//
// # Error Handling
//
// All provider-specific errors are mapped to sentinel errors defined in this
// package (e.g. [ErrCardDeclined], [ErrInsufficientFunds], [ErrNotFound]).
// Use [errors.Is] for matching:
//
//	if errors.Is(err, gopay.ErrCardDeclined) {
//	    // handle declined card
//	}
package gopay
