// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package capture

import "errors"

// ErrNotSupported is returned on non-Linux platforms where capture is unavailable.
var ErrNotSupported = errors.New("capture: strata capture requires Linux")
