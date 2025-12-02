/**
 * Search index utilities for building and filtering FQDN and location indices
 */

import type { FQDNEntry, LocationEntry } from './types';

/**
 * Parses a JSON array from GeoJSON properties, handling both string and array formats
 */
function parseJsonArray(value: unknown): string[] {
	if (typeof value === 'string') {
		try {
			return JSON.parse(value);
		} catch {
			return [];
		}
	}
	if (Array.isArray(value)) {
		return value;
	}
	return [];
}

/**
 * Builds an index of all FQDNs from GeoJSON features for search
 * Each FQDN gets its own entry, sorted by lastSeenAt descending
 */
export function buildFQDNIndex(geojson: GeoJSON.FeatureCollection): FQDNEntry[] {
	const entries: FQDNEntry[] = [];

	for (const feature of geojson.features) {
		const props = feature.properties;
		const fqdns = parseJsonArray(props?.fqdns);
		const lastSeenAt = props?.last_seen_at ? new Date(props.last_seen_at) : new Date(0);

		for (const fqdn of fqdns) {
			entries.push({ fqdn, feature, lastSeenAt });
		}
	}

	// Sort by lastSeenAt descending (newest first)
	entries.sort((a, b) => b.lastSeenAt.getTime() - a.lastSeenAt.getTime());
	return entries;
}

/**
 * Builds an index of unique locations from GeoJSON features
 * Each feature (location) gets one entry with its root domain, sorted by lastSeenAt descending
 */
export function buildLocationIndex(geojson: GeoJSON.FeatureCollection): LocationEntry[] {
	const entries: LocationEntry[] = [];

	for (const feature of geojson.features) {
		const props = feature.properties;
		const rootDomains = parseJsonArray(props?.root_domains);
		const fqdns = parseJsonArray(props?.fqdns);
		const lastSeenAt = props?.last_seen_at ? new Date(props.last_seen_at) : new Date(0);

		// Use first root domain as representative
		const rootDomain = rootDomains[0] || fqdns[0] || 'unknown';
		entries.push({ rootDomain, feature, lastSeenAt, fqdnCount: fqdns.length });
	}

	// Sort by lastSeenAt descending (newest first)
	entries.sort((a, b) => b.lastSeenAt.getTime() - a.lastSeenAt.getTime());
	return entries;
}
