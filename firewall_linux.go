package libnetwork

import (
	"github.com/docker/libnetwork/iptables"
	"github.com/sirupsen/logrus"
)

const userChain = "DOCKER-USER"

// This chain allow users to configure firewall policies in a way that persists
// docker operations/restarts. Docker will not delete or modify any pre-existing
// rules from the DOCKER-USER filter chain.
func arrangeUserFilterRule() {
	iptable := iptables.GetIptable(iptables.IPv4)
	_, err := iptable.NewChain(userChain, iptables.Filter, false)
	if err != nil {
		logrus.Warnf("Failed to create %s chain: %v", userChain, err)
		return
	}

	if err = iptable.AddReturnRule(userChain); err != nil {
		logrus.Warnf("Failed to add the RETURN rule for %s: %v", userChain, err)
		return
	}

	err = iptable.EnsureJumpRule("FORWARD", userChain)
	if err != nil {
		logrus.Warnf("Failed to ensure the jump rule for %s: %v", userChain, err)
	}
}
