package pipeline

// The GitHub Actions the generated workflows use, each at a major that runs on Node.js 24 (Node 20 is
// deprecated on the runners). They live here so the next bump is one edit; the tests assert no workflow
// pins an older one.
const (
	actionCheckout = "actions/checkout@v7"
	actionSetupGo  = "actions/setup-go@v7"
	actionCache    = "actions/cache@v6"
	actionGolangci = "golangci/golangci-lint-action@v9"
	actionSetupUV  = "astral-sh/setup-uv@v10"
)

// checkoutStep is the first step of every generated job.
const checkoutStep = "      - name: Checkout\n        uses: " + actionCheckout + "\n"
