// API client with auth handling

export function getApiKey(): string | null {
	if (typeof sessionStorage === 'undefined') return null;
	return sessionStorage.getItem('admin_api_key');
}

export function setApiKey(key: string): void {
	sessionStorage.setItem('admin_api_key', key);
}

export function clearApiKey(): void {
	sessionStorage.removeItem('admin_api_key');
}

export class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

async function adminFetch(path: string, options: RequestInit = {}): Promise<Response> {
	const apiKey = getApiKey();
	if (!apiKey) {
		throw new ApiError(401, 'No API key');
	}

	const response = await fetch(path, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			'X-Admin-Key': apiKey,
			...options.headers
		}
	});

	if (!response.ok) {
		if (response.status === 401) {
			clearApiKey();
		}
		const data = await response.json().catch(() => ({ error: 'Request failed' }));
		throw new ApiError(response.status, data.error || 'Request failed');
	}

	return response;
}

// Types
export interface Scanner {
	id: string;
	name: string;
	created_at: string;
	last_heartbeat: string | null;
	active_batches: number;
	is_alive: boolean;
}

export interface NewScanner {
	id: string;
	name: string;
	token: string;
}

export interface DomainFileStats {
	total: number;
	pending: number;
	processing: number;
	complete: number;
}

export interface BatchQueueStats {
	pending: number;
	in_flight: number;
}

export interface CurrentFileProgress {
	filename: string;
	processed_lines: number;
	batches_created: number;
	batches_completed: number;
	progress_pct: number;
}

export interface Stats {
	total_loc_records: number;
	unique_root_domains_with_loc: number;
	active_scanners: number;
	domain_files: DomainFileStats;
	batch_queue: BatchQueueStats;
	current_file?: CurrentFileProgress;
}

// API functions

// Public stats (no auth required)
export async function getStats(): Promise<Stats> {
	const response = await fetch('/api/public/stats');
	if (!response.ok) {
		throw new ApiError(response.status, 'Failed to fetch stats');
	}
	return response.json();
}

// Scanner management
export async function listScanners(): Promise<Scanner[]> {
	const response = await adminFetch('/api/admin/clients');
	const data = await response.json();
	return data.clients || [];
}

export async function createScanner(name: string): Promise<NewScanner> {
	const response = await adminFetch('/api/admin/clients', {
		method: 'POST',
		body: JSON.stringify({ name })
	});
	return response.json();
}

export async function deleteScanner(id: string): Promise<void> {
	await adminFetch(`/api/admin/clients/${id}`, {
		method: 'DELETE'
	});
}

export async function verifyApiKey(key: string): Promise<boolean> {
	const response = await fetch('/api/admin/clients', {
		headers: { 'X-Admin-Key': key }
	});
	return response.ok;
}

// Admin actions
export async function discoverFiles(): Promise<{ files_discovered: number }> {
	const response = await adminFetch('/api/admin/discover-files', {
		method: 'POST'
	});
	return response.json();
}

export async function resetScan(): Promise<{ files_reset: number }> {
	const response = await adminFetch('/api/admin/reset-scan', {
		method: 'POST'
	});
	return response.json();
}

export async function submitManualScan(domains: string[]): Promise<{ domains_queued: number }> {
	const response = await adminFetch('/api/admin/manual-scan', {
		method: 'POST',
		body: JSON.stringify({ domains })
	});
	return response.json();
}
