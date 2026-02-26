package main

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
	"github.com/tyler-smith/go-bip39"
)

type Wallet struct {
	Address        common.Address
	PrivateKey     *ecdsa.PrivateKey
	DerivationPath string
}

// GenerateMnemonic generates a new mnemonic phrase
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128) // 128 bits = 12 words
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	return mnemonic, nil
}

// DeriveWalletsFromMnemonic derives multiple wallets from a single mnemonic
func DeriveWalletsFromMnemonic(mnemonic string, count int) ([]*Wallet, error) {
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to create HD wallet: %w", err)
	}

	wallets := make([]*Wallet, 0, count)

	for i := 0; i < count; i++ {
		// Standard Ethereum derivation path: m/44'/60'/0'/0/i
		path := hdwallet.MustParseDerivationPath(fmt.Sprintf("m/44'/60'/0'/0/%d", i))
		
		account, err := wallet.Derive(path, false)
		if err != nil {
			return nil, fmt.Errorf("failed to derive account %d: %w", i, err)
		}

		privateKey, err := wallet.PrivateKey(account)
		if err != nil {
			return nil, fmt.Errorf("failed to get private key for account %d: %w", i, err)
		}

		w := &Wallet{
			Address:        account.Address,
			PrivateKey:     privateKey,
			DerivationPath: path.String(),
		}

		wallets = append(wallets, w)
	}

	return wallets, nil
}

// CreateWalletsFromMultipleMnemonics creates wallets from multiple mnemonics
func CreateWalletsFromMultipleMnemonics(mnemonicCount, walletsPerMnemonic int) ([][]*Wallet, []string, error) {
	allWallets := make([][]*Wallet, 0, mnemonicCount)
	mnemonics := make([]string, 0, mnemonicCount)

	for i := 0; i < mnemonicCount; i++ {
		mnemonic, err := GenerateMnemonic()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate mnemonic %d: %w", i, err)
		}

		mnemonics = append(mnemonics, mnemonic)

		wallets, err := DeriveWalletsFromMnemonic(mnemonic, walletsPerMnemonic)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to derive wallets from mnemonic %d: %w", i, err)
		}

		allWallets = append(allWallets, wallets)
	}

	return allWallets, mnemonics, nil
}

// GetPublicAddress returns the Ethereum address from a private key
func GetPublicAddress(privateKey *ecdsa.PrivateKey) common.Address {
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}
	}

	return crypto.PubkeyToAddress(*publicKeyECDSA)
}

// ParseDerivationPath parses a derivation path string
func ParseDerivationPath(path string) (accounts.DerivationPath, error) {
	return hdwallet.ParseDerivationPath(path)
}
