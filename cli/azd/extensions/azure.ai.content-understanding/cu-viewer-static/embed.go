// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewerstatic

import "embed"

//go:embed index.html all:assets LICENSE.txt THIRD-PARTY-NOTICES.txt
var Files embed.FS
