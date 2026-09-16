import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import type { ReactNode } from 'react';

import styles from './index.module.css';

type Feature = {
  readonly title: string;
  readonly description: string;
};

type Capability = {
  readonly capability: string;
};

type ProviderRow = {
  readonly provider: string;
  readonly payments: boolean;
  readonly refunds: boolean;
  readonly customers: boolean;
  readonly paymentMethods: boolean;
  readonly setupIntents: boolean;
  readonly subscriptions: boolean;
  readonly invoices: boolean;
  readonly webhooks: boolean;
  readonly listing: boolean;
};

const FEATURES = [
  {
    title: 'Unified Interface',
    description: 'One API across Stripe, PayPal, and Razorpay',
  },
  {
    title: 'Dependency Isolation',
    description:
      'Each provider is its own Go module — only pull in the SDKs you use',
  },
  {
    title: 'Payments',
    description: 'Create, capture (automatic or manual), get, and cancel',
  },
  {
    title: 'Refunds',
    description: 'Full and partial refund processing',
  },
  {
    title: 'Customer Management',
    description: 'Create and manage customers (Stripe, Razorpay)',
  },
  {
    title: 'Payment Methods',
    description: 'Attach and manage saved payment methods',
  },
  {
    title: 'Setup Intents',
    description: 'Save a card for later, off-session charges (Stripe)',
  },
  {
    title: 'Subscriptions',
    description: 'Plans and recurring billing (Stripe, Razorpay)',
  },
  {
    title: 'Invoices',
    description: 'Read-only invoice retrieval with hosted invoice URLs',
  },
  {
    title: 'Webhooks',
    description: 'Signature-verified, normalized events across providers',
  },
  {
    title: 'Cursor Pagination',
    description: 'A consistent List/paginate pattern across providers',
  },
  {
    title: 'Mock Provider',
    description: 'A full-featured in-memory provider for tests — no network',
  },
] as const satisfies readonly Feature[];

// Everything gopay provides over hand-rolling each provider's SDK. Kept in
// sync with the "Why gopay?" table in the repository README.
const CAPABILITIES = [
  { capability: 'One interface across Stripe, PayPal, and Razorpay' },
  { capability: 'Sentinel errors (errors.Is) instead of per-SDK error types' },
  {
    capability:
      'Builder + Validate() requests catch mistakes before the API call',
  },
  { capability: 'Cursor-based pagination, normalized across providers' },
  { capability: 'Webhook signature verification + normalized event kinds' },
  { capability: 'Mock provider for tests — no network, no API keys' },
  { capability: 'Dependency isolation — only the SDKs you actually use' },
] as const satisfies readonly Capability[];

// Kept in sync with the Supported Providers table in the repository README.
const PROVIDERS = [
  {
    provider: 'Stripe',
    payments: true,
    refunds: true,
    customers: true,
    paymentMethods: true,
    setupIntents: true,
    subscriptions: true,
    invoices: true,
    webhooks: true,
    listing: true,
  },
  {
    provider: 'PayPal',
    payments: true,
    refunds: true,
    customers: false,
    paymentMethods: false,
    setupIntents: false,
    subscriptions: false,
    invoices: false,
    webhooks: true,
    listing: false,
  },
  {
    provider: 'Razorpay',
    payments: true,
    refunds: true,
    customers: true,
    paymentMethods: false,
    setupIntents: false,
    subscriptions: true,
    invoices: true,
    webhooks: true,
    listing: true,
  },
] as const satisfies readonly ProviderRow[];

const INSTALL_COMMAND = 'go get github.com/KARTIKrocks/gopay';

function Mark({ value }: { readonly value: boolean }): ReactNode {
  if (value) {
    return (
      <>
        <span className={styles.check} aria-hidden="true">
          ✓
        </span>
        <span className={styles.srOnly}>Supported</span>
      </>
    );
  }
  return (
    <>
      <span className={styles.dash} aria-hidden="true">
        –
      </span>
      <span className={styles.srOnly}>Not supported</span>
    </>
  );
}

function Hero(): ReactNode {
  return (
    <header className={styles.hero}>
      <div className="container">
        <h1 className={styles.title}>
          Stripe, PayPal, and Razorpay behind one Go interface
        </h1>
        <p className={styles.subtitle}>
          gopay is a unified payment-processing library for Go —
          provider-agnostic types, sentinel errors, and a mock provider for
          tests. Each provider ships as its own Go module, so importing one
          never pulls in the SDKs of the others.
        </p>

        <div className={styles.buttons}>
          <Link
            className="button button--primary button--lg"
            to="/docs/getting-started">
            Get Started
          </Link>
          <Link
            className="button button--secondary button--lg"
            to="https://pkg.go.dev/github.com/KARTIKrocks/gopay">
            API Reference
          </Link>
        </div>

        <div className={styles.install}>
          <span className={styles.prompt} aria-hidden="true">
            $
          </span>
          <code>{INSTALL_COMMAND}</code>
        </div>
      </div>
    </header>
  );
}

function Features(): ReactNode {
  return (
    <section className="container" aria-label="Features">
      <div className={styles.features}>
        {FEATURES.map((feature) => (
          <article key={feature.title} className={styles.card}>
            <h2>{feature.title}</h2>
            <p>{feature.description}</p>
          </article>
        ))}
      </div>
    </section>
  );
}

function WhyGopay(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <h2 className={styles.sectionTitle}>Why gopay?</h2>
        <p className={styles.sectionLead}>
          Calling a payment provider's SDK directly gets you a working
          integration with exactly one provider. Everything past that — the
          parts that turn "I can charge a card with Stripe" into "I can swap in
          Razorpay for a region without rewriting my checkout code" — is what
          gopay provides.
        </p>

        <div className={styles.tableScroll}>
          <table className={styles.compare}>
            <thead>
              <tr>
                <th scope="col">Capability</th>
              </tr>
            </thead>
            <tbody>
              {CAPABILITIES.map(({ capability }) => (
                <tr key={capability}>
                  <th scope="row">
                    <span className={styles.check} aria-hidden="true">
                      ✓
                    </span>{' '}
                    {capability}
                  </th>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function SupportedProviders(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <h2 className={styles.sectionTitle}>Supported Providers</h2>
        <p className={styles.sectionLead}>
          Providers only implement what they support — PayPal's Orders API has
          no customer or list endpoint, and only Stripe implements setup
          intents. See the <Link to="/docs/">docs</Link> for provider-specific
          quirks.
        </p>

        <div className={styles.tableScroll}>
          <table className={styles.compare}>
            <thead>
              <tr>
                <th scope="col">Provider</th>
                <th scope="col">Payments</th>
                <th scope="col">Refunds</th>
                <th scope="col">Customers</th>
                <th scope="col">Payment Methods</th>
                <th scope="col">Setup Intents</th>
                <th scope="col">Subscriptions</th>
                <th scope="col">Invoices</th>
                <th scope="col">Webhooks</th>
                <th scope="col">Listing</th>
              </tr>
            </thead>
            <tbody>
              {PROVIDERS.map((row) => (
                <tr key={row.provider}>
                  <th scope="row">{row.provider}</th>
                  <td>
                    <Mark value={row.payments} />
                  </td>
                  <td>
                    <Mark value={row.refunds} />
                  </td>
                  <td>
                    <Mark value={row.customers} />
                  </td>
                  <td>
                    <Mark value={row.paymentMethods} />
                  </td>
                  <td>
                    <Mark value={row.setupIntents} />
                  </td>
                  <td>
                    <Mark value={row.subscriptions} />
                  </td>
                  <td>
                    <Mark value={row.invoices} />
                  </td>
                  <td>
                    <Mark value={row.webhooks} />
                  </td>
                  <td>
                    <Mark value={row.listing} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();

  return (
    <Layout
      title={siteConfig.tagline}
      description="A unified payment-processing library for Go with support for Stripe, PayPal, and Razorpay.">
      <Hero />
      <main>
        <Features />
        <WhyGopay />
        <SupportedProviders />
      </main>
    </Layout>
  );
}
