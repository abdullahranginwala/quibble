// Package store defines the CommentStore interface and its reference
// implementation: git-native thread files under .quibble/comments/.
// Cloud adapters (Cloudflare D1, DynamoDB, ...) implement the same
// interface and are tested against a shared conformance suite.
// See DESIGN.md §3 and §9.
package store
