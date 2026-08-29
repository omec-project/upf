<!--
SPDX-FileCopyrightText: 2016-2017, Nefeli Networks, Inc.
SPDX-FileCopyrightText: 2017, The Regents of the University of California.
SPDX-License-Identifier: BSD-3-Clause
-->

# Resume Hooks
Resume hooks allow you to run arbitrary code immediately before a worker is
resumed. They can be configured with the `ConfigureResumeHook()` RPC.

The API is simple, similar to that of `GateHook`. All you need to do is:

- Define `YourHook::kName`. This must be unique across all other resume hooks.
- Define `YourHook::kPriority` (lower values get higher priority). Ties are broken by hook name in increasing lexicographical order.
- Include `ResumeHook(kName, kPriority)` in `YourHook`'s initializer list.
- Define `void YourHook::Run()`.
- Define `void YourHook::Init(const bess::pb::YourHookArg &)`.
- Include `ADD_RESUME_HOOK(YourHook)` at the bottom of `resume_hooks/your_hook.cc`.
