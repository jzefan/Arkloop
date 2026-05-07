//go:build !windows || !desktop

package accountapi

import "arkloop/services/shared/napcat"

func wireNapCatOneBotProvider(_ *napcat.Manager) {}
