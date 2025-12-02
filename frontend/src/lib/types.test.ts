import { describe, it, expect } from 'vitest';
import { isFQDNEntry } from './types';
import type { FQDNEntry, LocationEntry } from './types';

describe('isFQDNEntry', () => {
	it('returns true for FQDNEntry objects', () => {
		const entry: FQDNEntry = {
			fqdn: 'example.com',
			feature: {
				type: 'Feature',
				geometry: { type: 'Point', coordinates: [0, 0] },
				properties: {}
			},
			lastSeenAt: new Date()
		};
		expect(isFQDNEntry(entry)).toBe(true);
	});

	it('returns false for LocationEntry objects', () => {
		const entry: LocationEntry = {
			rootDomain: 'example.com',
			feature: {
				type: 'Feature',
				geometry: { type: 'Point', coordinates: [0, 0] },
				properties: {}
			},
			lastSeenAt: new Date(),
			fqdnCount: 5
		};
		expect(isFQDNEntry(entry)).toBe(false);
	});
});
