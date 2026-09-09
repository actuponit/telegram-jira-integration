# Mela App — App Context

Mela App is the consumer mobile app for Mela, a cross-border payments and
remittance product. It is a Flutter app (iOS and Android), built with the
BLoC state-management pattern. Reporters may mention "BLoC error" or a
BLoC/Cubit name verbatim when pasting a crash or debug overlay — treat that
as identifying the app, not as a diagnosis.

## Features

- **Wallet** — the user's balance across supported currencies, with
  transaction history and balance top-up.
- **Transfer to Bank** — sending money from the wallet to a linked bank
  account, including transfer limits and delivery-time estimates.
- **Transfer to Someone** — peer-to-peer transfers to another Mela user or
  contact.
- **Request Money** — generating a request for another person to pay the
  user.
- **Exchange Rate** — live and remittance-specific exchange rates shown
  before a cross-border transfer is confirmed.
- **Card** — the Mela card: viewing card details, freezing/unfreezing, and
  card-linked spend history.
- **Merchant Payment** — paying a merchant directly from the wallet,
  typically via QR code.
- **Merchant Connect** — linking or onboarding a merchant account so it can
  receive Mela payments.
- **Ecommerce Payment** — checkout flow for paying an online store through
  Mela.
- **Crypto Receipt** — receiving funds that originate as a crypto payment,
  shown as a receipt in the app.
- **Chat** — in-app conversations, including support conversations with the
  Mela team.
- **Identity / KYC** — identity verification during onboarding or when a
  higher transaction limit requires re-verification, powered by Onfido
  (document capture and liveness check).
- **Compliance** — screening and additional-information prompts a user may
  see when a transfer or account activity requires review.
- **Contacts** — the user's saved contacts for sending or requesting money.
- **Location** — finding nearby agents, branches, or cash pickup/drop-off
  points.
- **Notifications** — push and in-app notifications for transaction status,
  security events, and account updates.
- **Payment Methods** — the user's linked funding sources: bank accounts
  (linked via Plaid) and cards (processed via Stripe).
- **Pincode** — the app's PIN and biometric unlock used to secure access
  and authorize sensitive actions.
- **User Profile** — account details, personal information, and settings.
- **Fee** — the fee breakdown shown before a transfer or payment is
  confirmed.
- **QR Scanner** — scanning a QR code to pay a merchant or add a contact.

## Third-party services and platforms

- **Flutter** — the cross-platform framework the app is built on.
- **BLoC** — the state-management pattern used throughout the app; its
  error/exception names can surface in crash reports and screenshots.
- **Onfido** — identity document capture and liveness verification during
  KYC.
- **Stripe** — card payment processing and card-linked funding.
- **Plaid** — linking and verifying external bank accounts.
- **Firebase** — Crashlytics (crash reporting), Analytics, Performance
  Monitoring, Remote Config, and push messaging.
- **Sentry** — error monitoring.
- **Mixpanel** — product analytics.

## Boundary

This context describes what the Mela app *has*, not how any of it is
built. It exists so the drafting model can recognize product shorthand a
reporter uses — "the Onfido screen froze," "Plaid didn't connect," "the OTC
agent list is empty" — and label and title an Issue using the team's own
vocabulary. It is not a map of files, modules, or internal routing, and it
must never be used to guess which file, screen, or service caused a
problem. If a report doesn't name a feature or vendor from this list, don't
force one — describe only what the reporter actually said.
