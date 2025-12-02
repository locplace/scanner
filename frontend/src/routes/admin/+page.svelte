<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		getApiKey,
		setApiKey,
		clearApiKey,
		verifyApiKey,
		listScanners,
		createScanner,
		deleteScanner,
		getStats,
		discoverFiles,
		resetScan,
		submitManualScan,
		ApiError,
		type Scanner,
		type NewScanner,
		type Stats
	} from '$lib/api';

	let authenticated = $state(false);
	let apiKeyInput = $state('');
	let authError = $state('');

	// Stats state
	let stats = $state<Stats | null>(null);
	let statsLoading = $state(false);
	let statsError = $state('');

	// Scanners state
	let scanners = $state<Scanner[]>([]);
	let scannersLoading = $state(false);
	let scannersError = $state('');
	let newScannerName = $state('');
	let newScannerResult = $state<NewScanner | null>(null);
	let createScannerError = $state('');

	// Admin actions state
	let actionLoading = $state(false);
	let actionResult = $state('');
	let actionError = $state('');

	// Manual scan state
	let manualScanDomains = $state('');
	let manualScanLoading = $state(false);
	let manualScanResult = $state('');
	let manualScanError = $state('');

	// Auto-refresh interval
	let refreshInterval: ReturnType<typeof setInterval> | null = null;

	onMount(() => {
		if (getApiKey()) {
			authenticated = true;
			loadData();
			startAutoRefresh();
		}
	});

	onDestroy(() => {
		stopAutoRefresh();
	});

	function startAutoRefresh() {
		refreshInterval = setInterval(() => {
			if (authenticated) {
				loadStats();
				loadScanners();
			}
		}, 5000);
	}

	function stopAutoRefresh() {
		if (refreshInterval) {
			clearInterval(refreshInterval);
			refreshInterval = null;
		}
	}

	async function loadData() {
		await Promise.all([loadStats(), loadScanners()]);
	}

	async function loadStats() {
		statsLoading = true;
		statsError = '';
		try {
			stats = await getStats();
		} catch (e) {
			statsError = e instanceof Error ? e.message : 'Failed to load stats';
		} finally {
			statsLoading = false;
		}
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
			startAutoRefresh();
		} else {
			authError = 'Invalid API key';
		}
	}

	function logout() {
		clearApiKey();
		authenticated = false;
		scanners = [];
		stats = null;
		stopAutoRefresh();
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
				stopAutoRefresh();
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
				stopAutoRefresh();
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
				stopAutoRefresh();
			} else {
				alert(e instanceof Error ? e.message : 'Failed to delete scanner');
			}
		}
	}

	// Admin actions
	async function handleDiscoverFiles() {
		actionLoading = true;
		actionResult = '';
		actionError = '';

		try {
			const result = await discoverFiles();
			actionResult = `Discovered ${result.files_discovered} new file(s)`;
			loadStats();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
				stopAutoRefresh();
			} else {
				actionError = e instanceof Error ? e.message : 'Failed to discover files';
			}
		} finally {
			actionLoading = false;
		}
	}

	async function handleResetScan() {
		if (
			!confirm('Reset all scanning progress? This will reset all domain files to pending status.')
		)
			return;

		actionLoading = true;
		actionResult = '';
		actionError = '';

		try {
			const result = await resetScan();
			actionResult = `Reset ${result.files_reset} file(s) to pending`;
			loadStats();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
				stopAutoRefresh();
			} else {
				actionError = e instanceof Error ? e.message : 'Failed to reset scan';
			}
		} finally {
			actionLoading = false;
		}
	}

	async function handleManualScan() {
		const domains = manualScanDomains
			.split('\n')
			.map((d) => d.trim())
			.filter((d) => d && !d.startsWith('#'));

		if (domains.length === 0) {
			manualScanError = 'Please enter at least one domain';
			return;
		}

		manualScanLoading = true;
		manualScanResult = '';
		manualScanError = '';

		try {
			const result = await submitManualScan(domains);
			manualScanResult = `Queued ${result.domains_queued} domain(s) for scanning`;
			manualScanDomains = '';
			loadStats();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				authenticated = false;
				stopAutoRefresh();
			} else {
				manualScanError = e instanceof Error ? e.message : 'Failed to queue domains';
			}
		} finally {
			manualScanLoading = false;
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

		<section class="stats-section">
			<h2>Scanning Progress</h2>

			{#if statsLoading && !stats}
				<p>Loading...</p>
			{:else if statsError}
				<p class="error">{statsError}</p>
			{:else if stats}
				<div class="stats-grid">
					<div class="stat-card">
						<div class="stat-value">{formatNumber(stats.total_loc_records)}</div>
						<div class="stat-label">LOC Records</div>
					</div>
					<div class="stat-card">
						<div class="stat-value">{formatNumber(stats.unique_root_domains_with_loc)}</div>
						<div class="stat-label">Unique Domains</div>
					</div>
					<div class="stat-card">
						<div class="stat-value">{stats.active_scanners}</div>
						<div class="stat-label">Active Scanners</div>
					</div>
				</div>

				<h3>Domain Files</h3>
				<div class="progress-bar-container">
					<div
						class="progress-bar"
						style="width: {stats.domain_files.total > 0
							? (stats.domain_files.complete / stats.domain_files.total) * 100
							: 0}%"
					></div>
				</div>
				<div class="file-stats">
					<span class="file-stat pending">Pending: {formatNumber(stats.domain_files.pending)}</span>
					<span class="file-stat processing"
						>Processing: {formatNumber(stats.domain_files.processing)}</span
					>
					<span class="file-stat complete"
						>Complete: {formatNumber(stats.domain_files.complete)}</span
					>
					<span class="file-stat total">Total: {formatNumber(stats.domain_files.total)}</span>
				</div>

				<h3>Batch Queue</h3>
				<div class="batch-stats">
					<span class="batch-stat">Pending: {formatNumber(stats.batch_queue.pending)}</span>
					<span class="batch-stat">In Flight: {formatNumber(stats.batch_queue.in_flight)}</span>
				</div>

				{#if stats.current_file}
					<h3>Current File</h3>
					<div class="current-file">
						<div class="filename">{stats.current_file.filename}</div>
						<div class="progress-bar-container small">
							<div class="progress-bar" style="width: {stats.current_file.progress_pct}%"></div>
						</div>
						<div class="file-progress">
							<span>Lines: {formatNumber(stats.current_file.processed_lines)}</span>
							<span
								>Batches: {formatNumber(stats.current_file.batches_completed)} / {formatNumber(
									stats.current_file.batches_created
								)}</span
							>
							<span>{stats.current_file.progress_pct.toFixed(1)}%</span>
						</div>
					</div>
				{/if}
			{/if}
		</section>

		<section>
			<h2>Admin Actions</h2>
			<div class="action-buttons">
				<button onclick={handleDiscoverFiles} disabled={actionLoading}>
					{actionLoading ? 'Working...' : 'Discover Files'}
				</button>
				<button class="danger" onclick={handleResetScan} disabled={actionLoading}>
					{actionLoading ? 'Working...' : 'Reset Scan'}
				</button>
			</div>
			{#if actionError}
				<p class="error">{actionError}</p>
			{/if}
			{#if actionResult}
				<p class="success">{actionResult}</p>
			{/if}
		</section>

		<section>
			<h2>Manual Scan</h2>
			<p class="section-description">
				Queue specific domains for scanning. Enter one domain per line.
			</p>
			<form
				onsubmit={(e) => {
					e.preventDefault();
					handleManualScan();
				}}
			>
				<textarea
					bind:value={manualScanDomains}
					placeholder={"example.com\nsubdomain.example.org\n# Comments are ignored"}
					rows="5"
					class="domains-input"
				></textarea>
				<button type="submit" disabled={manualScanLoading || !manualScanDomains.trim()}>
					{manualScanLoading ? 'Queuing...' : 'Queue for Scanning'}
				</button>
			</form>
			{#if manualScanError}
				<p class="error">{manualScanError}</p>
			{/if}
			{#if manualScanResult}
				<p class="success">{manualScanResult}</p>
			{/if}
		</section>

		<section>
			<h2>Scanners</h2>

			{#if scannersLoading && scanners.length === 0}
				<p>Loading...</p>
			{:else if scannersError}
				<p class="error">{scannersError}</p>
			{:else if scanners.length === 0}
				<p class="muted">No scanners registered</p>
			{:else}
				<div class="table-wrapper">
					<table>
						<thead>
							<tr>
								<th>Name</th>
								<th>Status</th>
								<th>Active Batches</th>
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
									<td>{scanner.active_batches}</td>
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
				</div>
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

	h2 {
		margin-bottom: 1rem;
		color: #333;
	}

	h3 {
		margin-top: 1.5rem;
		margin-bottom: 0.75rem;
		font-size: 1rem;
		color: #666;
	}

	/* Stats grid */
	.stats-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.stat-card {
		background: #f8f9fa;
		padding: 1.25rem;
		border-radius: 8px;
		text-align: center;
	}

	.stat-value {
		font-size: 1.75rem;
		font-weight: 700;
		color: #3131dc;
	}

	.stat-label {
		font-size: 0.875rem;
		color: #666;
		margin-top: 0.25rem;
	}

	/* Progress bars */
	.progress-bar-container {
		background: #eee;
		border-radius: 4px;
		height: 24px;
		overflow: hidden;
		margin-bottom: 0.5rem;
	}

	.progress-bar-container.small {
		height: 12px;
	}

	.progress-bar {
		background: linear-gradient(90deg, #3131dc, #5050ff);
		height: 100%;
		transition: width 0.3s ease;
	}

	/* File stats */
	.file-stats {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
	}

	.file-stat {
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}

	.file-stat.pending {
		background: #fff3cd;
		color: #856404;
	}

	.file-stat.processing {
		background: #cce5ff;
		color: #004085;
	}

	.file-stat.complete {
		background: #d4edda;
		color: #155724;
	}

	.file-stat.total {
		background: #e2e3e5;
		color: #383d41;
	}

	/* Batch stats */
	.batch-stats {
		display: flex;
		gap: 1.5rem;
	}

	.batch-stat {
		font-size: 0.9rem;
		color: #333;
	}

	/* Current file */
	.current-file {
		background: #f0f4ff;
		padding: 1rem;
		border-radius: 8px;
		border: 1px solid #c5d5ff;
	}

	.current-file .filename {
		font-family: monospace;
		font-size: 0.875rem;
		color: #333;
		margin-bottom: 0.5rem;
		word-break: break-all;
	}

	.current-file .file-progress {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		font-size: 0.8rem;
		color: #666;
	}

	/* Action buttons */
	.action-buttons {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.action-buttons button {
		padding: 0.75rem 1.5rem;
	}

	.action-buttons button.danger {
		background: #dc3545;
	}

	.action-buttons button.danger:hover:not(:disabled) {
		background: #c82333;
	}

	/* Manual scan section */
	.section-description {
		color: #666;
		font-size: 0.9rem;
		margin-bottom: 1rem;
	}

	.domains-input {
		width: 100%;
		padding: 0.75rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-family: monospace;
		font-size: 0.875rem;
		resize: vertical;
		margin-bottom: 0.75rem;
		box-sizing: border-box;
	}

	.domains-input:focus {
		outline: none;
		border-color: #3131dc;
	}

	/* Table */
	.table-wrapper {
		overflow-x: auto;
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

		.stats-grid {
			grid-template-columns: 1fr 1fr;
		}

		.stat-value {
			font-size: 1.5rem;
		}

		/* Tables scroll via wrapper */
		table {
			white-space: nowrap;
		}

		th,
		td {
			padding: 0.5rem;
			font-size: 0.875rem;
		}

		/* Stack form elements */
		.inline-form {
			flex-direction: column;
		}

		.inline-form input {
			width: 100%;
		}

		.inline-form button {
			width: 100%;
		}

		.action-buttons {
			flex-direction: column;
		}

		.action-buttons button {
			width: 100%;
		}

		.file-stats {
			flex-direction: column;
			gap: 0.5rem;
		}

		.batch-stats {
			flex-direction: column;
			gap: 0.5rem;
		}
	}

	@media (max-width: 480px) {
		.admin {
			padding: 0.75rem;
		}

		h2 {
			font-size: 1.25rem;
		}

		.stats-grid {
			grid-template-columns: 1fr;
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

	/* Dark mode */
	@media (prefers-color-scheme: dark) {
		.admin {
			background: #121212;
			color: #e0e0e0;
		}

		.login-container input {
			background: #2a2a2a;
			border-color: #444;
			color: #e0e0e0;
		}

		.login-container button {
			background: #4a4aff;
		}

		.login-container button:hover {
			background: #3a3aee;
		}

		header {
			border-bottom-color: #333;
		}

		h2 {
			color: #e0e0e0;
		}

		h3 {
			color: #aaa;
		}

		.stat-card {
			background: #1e1e1e;
		}

		.stat-value {
			color: #6a6aff;
		}

		.progress-bar-container {
			background: #333;
		}

		.file-stat.pending {
			background: #3d3500;
			color: #ffd700;
		}

		.file-stat.processing {
			background: #002d5c;
			color: #6ab7ff;
		}

		.file-stat.complete {
			background: #1a3d1a;
			color: #6bcf6b;
		}

		.file-stat.total {
			background: #2a2a2a;
			color: #ccc;
		}

		.current-file {
			background: #1a1a2e;
			border-color: #333366;
		}

		.current-file .filename {
			color: #e0e0e0;
		}

		.current-file .file-progress {
			color: #aaa;
		}

		.section-description {
			color: #aaa;
		}

		.domains-input {
			background: #2a2a2a;
			border-color: #444;
			color: #e0e0e0;
		}

		.domains-input:focus {
			border-color: #6a6aff;
		}

		table {
			background: #1e1e1e;
		}

		th {
			color: #aaa;
		}

		th,
		td {
			border-bottom-color: #333;
		}

		.status {
			background: #3d1a1a;
			color: #ff6b6b;
		}

		.status.active {
			background: #1a3d1a;
			color: #6bcf6b;
		}

		input[type='text'],
		input[type='password'] {
			background: #2a2a2a;
			border-color: #444;
			color: #e0e0e0;
		}

		button {
			background: #4a4aff;
		}

		button:hover:not(:disabled) {
			background: #3a3aee;
		}

		button:disabled {
			background: #444;
		}

		button.delete {
			background: #c00;
		}

		button.delete:hover {
			background: #a00;
		}

		.logout {
			background: #555;
		}

		.action-buttons button.danger {
			background: #c9302c;
		}

		.action-buttons button.danger:hover:not(:disabled) {
			background: #ac2925;
		}

		.error {
			color: #ff6b6b;
		}

		.success {
			color: #6bcf6b;
		}

		.muted {
			color: #777;
		}

		.token-result {
			background: #2a2a1a;
			border-color: #665500;
		}

		.token-result code {
			background: #1e1e1e;
			border-color: #444;
			color: #e0e0e0;
		}
	}
</style>
