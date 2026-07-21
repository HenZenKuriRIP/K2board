package gateways

// Mock payment gateway has been permanently removed from the registry.
// Historical orders may still store payment_method="mock"; they cannot be
// fulfilled via confirm-mock or public notify.
//
// Free (zero-amount) plans use Checkout → IntentCompleted without mock.
