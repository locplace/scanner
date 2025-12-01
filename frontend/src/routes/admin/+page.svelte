<script lang="ts">
	import { onMount } from 'svelte';
	import {
		getApiKey,
		setApiKey,
		clearApiKey,
		verifyApiKey,
		listScanners,
		createScanner,
		deleteScanner,
		listDomainSets,
		createDomainSet,
		deleteDomainSet,
		addDomainsToSet,
		bumpDomainSet,
		ApiError,
		type Scanner,
		type NewScanner,
		type DomainSet,
		type NewDomainSet
	} from '$lib/api';

	let authenticated = $state(false);
	let apiKeyInput = $state('');
	let authError = $state('');

	// Scanners state
	let scanners = $state<Scanner[]>([]);
	let scannersLoading = $state(false);
	let scannersError = $state('');
	let newScannerName = $state('');
	let newScannerResult = $state<NewScanner | null>(null);
	let createScannerError = $state('');

	// Domain sets state
	let domainSets = $state<DomainSet[]>([]);
	let domainSetsLoading = $state(false);
	let domainSetsError = $state('');
	let newSetName = $state('');
	let newSetSource = $state('');
	let newSetResult = $state<NewDomainSet | null>(null);
	let createSetError = $state('');

	// Domain upload state
	let selectedSetId = $state('');
	let domainsInput = $state('');
	let domainsResult = $state<{ inserted: number; duplicates: number } | null>(null);
	let domainsError = $state('');

	onMount(() => {
		if (getApiKey()) {
			authenticated = true;
			loadData();
		}
	});

	async function loadData() {
		await Promise.all([loadScanners(), loadDomainSets()]);
	}

	async function login() {
		authError = '';
		if (!apiKeyInput.trim()) {
			authError = 'API key is required';
			return;
		}

		const valid = await verifyApiKey(apiKeyInput.trim());
		if (valid) {
			setApiKey(apiKeyInput.trim());
			authenticated = true;
			apiKeyInput = '';
			loadData();
		} else {
			authError = 'Invalid API key';
		}
	}

	function logout() {
		clearApiKey();
		authenticated = false;
		scanners = [];
		domainSets = [];
	}

	// Scanners
	async function loadScanners() {
		scannersLoading = true;
		scannersError = '';
		try {
			scanners = await listScanners();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				scannersError = e instanceof Error ? e.message : 'Failed to load scanners';
			}
		} finally {
			scannersLoading = false;
		}
	}

	async function handleCreateScanner() {
		createScannerError = '';
		newScannerResult = null;
		if (!newScannerName.trim()) {
			createScannerError = 'Name is required';
			return;
		}

		try {
			newScannerResult = await createScanner(newScannerName.trim());
			newScannerName = '';
			loadScanners();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				createScannerError = e instanceof Error ? e.message : 'Failed to create scanner';
			}
		}
	}

	async function handleDeleteScanner(id: string, name: string) {
		if (!confirm(`Delete scanner "${name}"?`)) return;

		try {
			await deleteScanner(id);
			loadScanners();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				alert(e instanceof Error ? e.message : 'Failed to delete scanner');
			}
		}
	}

	// Domain Sets
	async function loadDomainSets() {
		domainSetsLoading = true;
		domainSetsError = '';
		try {
			domainSets = await listDomainSets();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				domainSetsError = e instanceof Error ? e.message : 'Failed to load domain sets';
			}
		} finally {
			domainSetsLoading = false;
		}
	}

	async function handleCreateDomainSet() {
		createSetError = '';
		newSetResult = null;
		if (!newSetName.trim()) {
			createSetError = 'Name is required';
			return;
		}
		if (!newSetSource.trim()) {
			createSetError = 'Source is required';
			return;
		}

		try {
			newSetResult = await createDomainSet(newSetName.trim(), newSetSource.trim());
			newSetName = '';
			newSetSource = '';
			loadDomainSets();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				createSetError = e instanceof Error ? e.message : 'Failed to create domain set';
			}
		}
	}

	async function handleDeleteDomainSet(id: string, name: string) {
		if (!confirm(`Delete domain set "${name}"? Domains will remain but lose their set association.`))
			return;

		try {
			await deleteDomainSet(id);
			if (selectedSetId === id) {
				selectedSetId = '';
			}
			loadDomainSets();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				alert(e instanceof Error ? e.message : 'Failed to delete domain set');
			}
		}
	}

	async function handleBumpDomainSet(id: string, name: string) {
		try {
			const result = await bumpDomainSet(id);
			alert(`Bumped ${result.bumped} unscanned domain(s) in "${name}" to front of queue`);
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				alert(e instanceof Error ? e.message : 'Failed to bump domain set');
			}
		}
	}

	// Domain upload
	async function handleAddDomains() {
		domainsError = '';
		domainsResult = null;

		if (!selectedSetId) {
			domainsError = 'Select a domain set first';
			return;
		}

		const domains = domainsInput
			.split(/[\n,]+/)
			.map((d) => d.trim())
			.filter((d) => d.length > 0);

		if (domains.length === 0) {
			domainsError = 'Enter at least one domain';
			return;
		}

		try {
			domainsResult = await addDomainsToSet(selectedSetId, domains);
			domainsInput = '';
			loadDomainSets();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
			} else {
				domainsError = e instanceof Error ? e.message : 'Failed to add domains';
			}
		}
	}

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const date = new Date(dateStr);
		return date.toLocaleString();
	}

	function formatNumber(n: number): string {
		return n.toLocaleString();
	}
</script>

<svelte:head>
	<title>Admin - LOC Place</title>
</svelte:head>

<div class="admin">
	{#if !authenticated}
		<div class="login-container">
			<h1>Admin Login</h1>
			<form
				onsubmit={(e) => {
					e.preventDefault();
					login();
				}}
			>
				<input
					type="password"
					bind:value={apiKeyInput}
					placeholder="Admin API Key"
					autocomplete="off"
				/>
				<button type="submit">Login</button>
			</form>
			{#if authError}
				<p class="error">{authError}</p>
			{/if}
		</div>
	{:else}
		<header>
			<h1>LOC Place Admin</h1>
			<button class="logout" onclick={logout}>Logout</button>
		</header>

		<section>
			<h2>Domain Sets</h2>
			<p class="section-description">
				Organize domains by source. Create a set, then add domains to it.
			</p>

			{#if domainSetsLoading}
				<p>Loading...</p>
			{:else if domainSetsError}
				<p class="error">{domainSetsError}</p>
			{:else if domainSets.length === 0}
				<p class="muted">No domain sets yet. Create one to start adding domains.</p>
			{:else}
				<table>
					<thead>
						<tr>
							<th>Name</th>
							<th>Source</th>
							<th>Domains</th>
							<th>Progress</th>
							<th>Created</th>
							<th>Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each domainSets as set}
							<tr>
								<td><strong>{set.name}</strong></td>
								<td class="source">{set.source}</td>
								<td>{formatNumber(set.total_domains)}</td>
								<td>
									{#if set.total_domains > 0}
										<span class="progress">
											{formatNumber(set.scanned_domains)} / {formatNumber(set.total_domains)}
											({Math.round((set.scanned_domains / set.total_domains) * 100)}%)
										</span>
									{:else}
										<span class="muted">-</span>
									{/if}
								</td>
								<td>{formatDate(set.created_at)}</td>
								<td class="actions">
									<button
										class="bump"
										onclick={() => handleBumpDomainSet(set.id, set.name)}
										title="Move unscanned domains to front of queue"
									>
										Bump
									</button>
									<button class="delete" onclick={() => handleDeleteDomainSet(set.id, set.name)}>
										Delete
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}

			<h3>Create Domain Set</h3>
			<form
				class="create-set-form"
				onsubmit={(e) => {
					e.preventDefault();
					handleCreateDomainSet();
				}}
			>
				<input type="text" bind:value={newSetName} placeholder="Name (e.g., .com zone file)" />
				<input type="text" bind:value={newSetSource} placeholder="Source (e.g., ICANN CZDS)" />
				<button type="submit">Create</button>
			</form>
			{#if createSetError}
				<p class="error">{createSetError}</p>
			{/if}
			{#if newSetResult}
				<p class="success">Created domain set "{newSetResult.name}"</p>
			{/if}
		</section>

		<section>
			<h2>Add Domains</h2>

			<div class="set-selector">
				<label for="domain-set">Domain Set:</label>
				<select id="domain-set" bind:value={selectedSetId}>
					<option value="">-- Select a domain set --</option>
					{#each domainSets as set}
						<option value={set.id}>{set.name} ({formatNumber(set.total_domains)} domains)</option>
					{/each}
				</select>
			</div>

			<form
				onsubmit={(e) => {
					e.preventDefault();
					handleAddDomains();
				}}
			>
				<textarea
					bind:value={domainsInput}
					placeholder="Enter domains (one per line or comma-separated)"
					rows="5"
					disabled={!selectedSetId}
				></textarea>
				<button type="submit" disabled={!selectedSetId}>Add Domains</button>
			</form>
			{#if domainsError}
				<p class="error">{domainsError}</p>
			{/if}
			{#if domainsResult}
				<p class="success">
					Added {domainsResult.inserted} domain(s)
					{#if domainsResult.duplicates > 0}
						({domainsResult.duplicates} duplicates skipped)
					{/if}
				</p>
			{/if}
		</section>

		<section>
			<h2>Scanners</h2>

			{#if scannersLoading}
				<p>Loading...</p>
			{:else if scannersError}
				<p class="error">{scannersError}</p>
			{:else if scanners.length === 0}
				<p class="muted">No scanners registered</p>
			{:else}
				<table>
					<thead>
						<tr>
							<th>Name</th>
							<th>Status</th>
							<th>Last Heartbeat</th>
							<th>Created</th>
							<th></th>
						</tr>
					</thead>
					<tbody>
						{#each scanners as scanner}
							<tr>
								<td>{scanner.name}</td>
								<td>
									<span class="status" class:active={scanner.is_alive}>
										{scanner.is_alive ? 'Active' : 'Inactive'}
									</span>
								</td>
								<td>{formatDate(scanner.last_heartbeat)}</td>
								<td>{formatDate(scanner.created_at)}</td>
								<td>
									<button
										class="delete"
										onclick={() => handleDeleteScanner(scanner.id, scanner.name)}
									>
										Delete
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}

			<h3>Add Scanner</h3>
			<form
				class="inline-form"
				onsubmit={(e) => {
					e.preventDefault();
					handleCreateScanner();
				}}
			>
				<input type="text" bind:value={newScannerName} placeholder="Scanner name" />
				<button type="submit">Create</button>
			</form>
			{#if createScannerError}
				<p class="error">{createScannerError}</p>
			{/if}
			{#if newScannerResult}
				<div class="token-result">
					<p><strong>Scanner created!</strong> Save this token - it won't be shown again:</p>
					<code>{newScannerResult.token}</code>
				</div>
			{/if}
		</section>
	{/if}
</div>

<style>
	.admin {
		max-width: 900px;
		margin: 0 auto;
		padding: 2rem;
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
	}

	.login-container {
		max-width: 300px;
		margin: 4rem auto;
		text-align: center;
	}

	.login-container input {
		width: 100%;
		padding: 0.75rem;
		margin-bottom: 1rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 1rem;
	}

	.login-container button {
		width: 100%;
		padding: 0.75rem;
		background: #3131dc;
		color: white;
		border: none;
		border-radius: 4px;
		font-size: 1rem;
		cursor: pointer;
	}

	.login-container button:hover {
		background: #2828b8;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 2rem;
		padding-bottom: 1rem;
		border-bottom: 1px solid #eee;
	}

	header h1 {
		margin: 0;
	}

	.logout {
		padding: 0.5rem 1rem;
		background: #666;
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
	}

	section {
		margin-bottom: 3rem;
	}

	.section-description {
		color: #666;
		margin-bottom: 1rem;
	}

	h2 {
		margin-bottom: 0.5rem;
		color: #333;
	}

	h3 {
		margin-top: 1.5rem;
		margin-bottom: 0.75rem;
		font-size: 1rem;
		color: #666;
	}

	table {
		width: 100%;
		border-collapse: collapse;
	}

	th,
	td {
		padding: 0.75rem;
		text-align: left;
		border-bottom: 1px solid #eee;
	}

	th {
		font-weight: 600;
		color: #666;
		font-size: 0.875rem;
	}

	.source {
		color: #666;
		font-size: 0.875rem;
	}

	.progress {
		font-size: 0.875rem;
		color: #333;
	}

	.status {
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
		font-weight: 600;
		background: #fee;
		color: #c00;
	}

	.status.active {
		background: #efe;
		color: #080;
	}

	.inline-form {
		display: flex;
		gap: 0.5rem;
	}

	.create-set-form {
		display: flex;
		gap: 0.5rem;
	}

	.create-set-form input {
		flex: 1;
	}

	.set-selector {
		margin-bottom: 1rem;
	}

	.set-selector label {
		display: block;
		margin-bottom: 0.5rem;
		font-weight: 500;
	}

	.set-selector select {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 1rem;
	}

	input[type='text'],
	input[type='password'] {
		padding: 0.5rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 1rem;
	}

	.inline-form input {
		flex: 1;
	}

	textarea {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 1rem;
		font-family: inherit;
		resize: vertical;
	}

	textarea:disabled {
		background: #f5f5f5;
		cursor: not-allowed;
	}

	button {
		padding: 0.5rem 1rem;
		background: #3131dc;
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 1rem;
	}

	button:hover:not(:disabled) {
		background: #2828b8;
	}

	button:disabled {
		background: #999;
		cursor: not-allowed;
	}

	button.delete {
		background: #c00;
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
	}

	button.delete:hover {
		background: #a00;
	}

	button.bump {
		background: #f90;
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
	}

	button.bump:hover {
		background: #e80;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.error {
		color: #c00;
		margin-top: 0.5rem;
	}

	.success {
		color: #080;
		margin-top: 0.5rem;
	}

	.muted {
		color: #999;
	}

	.token-result {
		margin-top: 1rem;
		padding: 1rem;
		background: #ffe;
		border: 1px solid #cc0;
		border-radius: 4px;
	}

	.token-result code {
		display: block;
		margin-top: 0.5rem;
		padding: 0.5rem;
		background: #fff;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-family: monospace;
		word-break: break-all;
	}

	/* Mobile responsiveness */
	@media (max-width: 768px) {
		.admin {
			padding: 1rem;
		}

		header {
			flex-direction: column;
			align-items: flex-start;
			gap: 1rem;
		}

		header h1 {
			font-size: 1.5rem;
		}

		/* Make tables scrollable */
		table {
			display: block;
			overflow-x: auto;
			white-space: nowrap;
		}

		th,
		td {
			padding: 0.5rem;
			font-size: 0.875rem;
		}

		/* Stack form elements */
		.create-set-form,
		.inline-form {
			flex-direction: column;
		}

		.create-set-form input,
		.inline-form input {
			width: 100%;
		}

		.create-set-form button,
		.inline-form button {
			width: 100%;
		}

		/* Stack action buttons */
		.actions {
			flex-direction: column;
			gap: 0.25rem;
		}

		.actions button {
			width: 100%;
		}
	}

	@media (max-width: 480px) {
		.admin {
			padding: 0.75rem;
		}

		h2 {
			font-size: 1.25rem;
		}

		th,
		td {
			padding: 0.375rem;
			font-size: 0.8rem;
		}

		button {
			padding: 0.5rem 0.75rem;
			font-size: 0.875rem;
		}
	}
</style>
