/**
 * Shared types for the LOC.place frontend
 */

/** Entry representing an individual FQDN for search results */
export interface FQDNEntry {
	fqdn: string;
	feature: GeoJSON.Feature;
	lastSeenAt: Date;
}

/** Entry representing a location (deduplicated by coordinates) for recent list */
export interface LocationEntry {
	rootDomain: string;
	feature: GeoJSON.Feature;
	lastSeenAt: Date;
	fqdnCount: number;
}

/** Union type for entries displayed in the search panel */
export type SearchEntry = FQDNEntry | LocationEntry;

/** Type guard to check if entry is an FQDNEntry */
export function isFQDNEntry(entry: SearchEntry): entry is FQDNEntry {
	return 'fqdn' in entry;
}

/** Public stats returned by the API */
export interface PublicStats {
	total_loc_records: number;
	unique_root_domains_with_loc: number;
}
