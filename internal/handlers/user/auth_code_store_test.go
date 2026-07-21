package user

import "testing"

func TestCodeStorePurposeIsolation(t *testing.T) {
	email := "iso-test@example.com"
	storeCode(codePurposeRegister, email, "111111")
	storeCode(codePurposeReset, email, "222222")

	if !verifyCode(codePurposeRegister, email, "111111") {
		t.Fatal("register code should verify")
	}
	// register code consumed
	if verifyCode(codePurposeRegister, email, "111111") {
		t.Fatal("register code should be one-time")
	}
	// reset still valid
	if !verifyCode(codePurposeReset, email, "222222") {
		t.Fatal("reset code should still verify after register consumed")
	}
}

func TestCodeStoreWrongPurposeFails(t *testing.T) {
	email := "wrong-purpose@example.com"
	storeCode(codePurposeReset, email, "333333")
	if verifyCode(codePurposeRegister, email, "333333") {
		t.Fatal("register purpose must not accept reset code")
	}
}

func TestVerifyCode_FailCountInvalidates(t *testing.T) {
	email := "fail-lock@example.com"
	storeCode(codePurposeRegister, email, "654321")
	for i := 0; i < maxCodeFails-1; i++ {
		if verifyCode(codePurposeRegister, email, "000000") {
			t.Fatal("wrong code must fail")
		}
	}
	// still present until max fails
	if !verifyCode(codePurposeRegister, email, "654321") {
		t.Fatal("correct code should work before max fails reached")
	}

	storeCode(codePurposeRegister, email, "654321")
	for i := 0; i < maxCodeFails; i++ {
		_ = verifyCode(codePurposeRegister, email, "000000")
	}
	// invalidated after max wrong attempts
	if verifyCode(codePurposeRegister, email, "654321") {
		t.Fatal("code must be wiped after max fails")
	}
}

func TestVerifyCode_HappyPathUnchanged(t *testing.T) {
	email := "happy@example.com"
	storeCode(codePurposeRegister, email, "999888")
	if !verifyCode(codePurposeRegister, email, "999888") {
		t.Fatal("first correct attempt must succeed")
	}
}
