package chainscanner

import (
	"strings"
	"sync"
)

// TrieNode represents a single character node in the Address Trie
type TrieNode struct {
	children map[rune]*TrieNode
	isEnd    bool
}

// AddressTrie is a highly concurrent Prefix-Tree (Trie) for Ethereum addresses.
// It allows O(L) complexity for checking if a deposit address belongs to the exchange.
type AddressTrie struct {
	root *TrieNode
	mu   sync.RWMutex
}

// NewAddressTrie creates a new in-memory Address Trie.
func NewAddressTrie() *AddressTrie {
	return &AddressTrie{
		root: &TrieNode{
			children: make(map[rune]*TrieNode),
		},
	}
}

// Add inserts a new deposit address into the Trie.
// Automatically converts the address to lowercase for case-insensitive matching.
func (t *AddressTrie) Add(address string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	address = strings.ToLower(address)
	curr := t.root
	for _, ch := range address {
		if _, exists := curr.children[ch]; !exists {
			curr.children[ch] = &TrieNode{
				children: make(map[rune]*TrieNode),
			}
		}
		curr = curr.children[ch]
	}
	curr.isEnd = true
}

// Contains checks if the given address exists in our deposit address database.
// O(L) time complexity where L is address length (constant 42 for EVM).
func (t *AddressTrie) Contains(address string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	address = strings.ToLower(address)
	curr := t.root
	for _, ch := range address {
		if node, exists := curr.children[ch]; exists {
			curr = node
		} else {
			return false
		}
	}
	return curr.isEnd
}
