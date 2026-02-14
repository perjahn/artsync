package main

import (
	"fmt"
	"log"

	"github.com/go-ldap/ldap/v3"
)

func queryldap(server, baseDN, filter, bindDN, bindPW string, attrs []string) ([]*ldap.Entry, error) {
	log.Printf("server: '%s', baseDN: '%s', filter: '%s', bindDN: '%s'\n", server, baseDN, filter, bindDN)

	ldapconn, err := ldap.DialURL(server)
	if err != nil {
		return nil, fmt.Errorf("failed to dial LDAP: %w", err)
	}
	defer ldapconn.Close()

	if bindDN != "" {
		if err = ldapconn.Bind(bindDN, bindPW); err != nil {
			return nil, fmt.Errorf("bind failed: %w", err)
		}
	}

	searchReq := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		attrs,
		nil,
	)

	sr, err := ldapconn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	log.Printf("Got %d entries, %d controls, %d referrals\n", len(sr.Entries), len(sr.Controls), len(sr.Referrals))

	return sr.Entries, nil
}
