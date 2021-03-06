// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"fmt"
	"net/http"

	"github.com/google/exposure-notifications-verification-server/pkg/controller/flash"
	"github.com/google/exposure-notifications-verification-server/pkg/database"

	"github.com/gorilla/context"
)

// GetUser gets the current logged in user from the request context. On an Error,
// a message is added to the context's flash, but no redirect/render decision is made.
func GetUser(w http.ResponseWriter, r *http.Request) (*database.User, error) {
	rawUser, ok := context.GetOk(r, "user")
	if !ok {
		flash.FromContext(w, r).Error("Unauthorized")
		return nil, fmt.Errorf("unauthorized")
	}
	user, ok := rawUser.(*database.User)
	if !ok {
		flash.FromContext(w, r).Error("internal error - you have been logged out.")
		return nil, fmt.Errorf("internal error")
	}
	return user, nil
}
