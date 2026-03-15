// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)
// IOReport + SMC implementation adapted from mactop (MIT).

#include "smc.h"
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <stdint.h>
#include <string.h>
#include <unistd.h>

typedef struct IOReportSubscriptionRef *IOReportSubscriptionRef;

extern CFDictionaryRef IOReportCopyChannelsInGroup(CFStringRef group,
	CFStringRef subgroup, uint64_t a, uint64_t b, uint64_t c);
extern void IOReportMergeChannels(CFDictionaryRef a, CFDictionaryRef b,
	CFTypeRef unused);
extern IOReportSubscriptionRef IOReportCreateSubscription(void *a,
	CFMutableDictionaryRef channels, CFMutableDictionaryRef *out, uint64_t d,
	CFTypeRef e);
extern CFDictionaryRef IOReportCreateSamples(IOReportSubscriptionRef sub,
	CFMutableDictionaryRef channels, CFTypeRef unused);
extern CFDictionaryRef IOReportCreateSamplesDelta(CFDictionaryRef a,
	CFDictionaryRef b, CFTypeRef unused);
extern int64_t IOReportSimpleGetIntegerValue(CFDictionaryRef item, int32_t idx);
extern CFStringRef IOReportChannelGetGroup(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetSubGroup(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetChannelName(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetUnitLabel(CFDictionaryRef item);
extern int32_t IOReportStateGetCount(CFDictionaryRef item);
extern CFStringRef IOReportStateGetNameForIndex(CFDictionaryRef item, int32_t idx);
extern int64_t IOReportStateGetResidency(CFDictionaryRef item, int32_t idx);

typedef struct {
	double gpuPowerW;
	double cpuPowerW;
	double anePowerW;
	double systemPowerW;
	double gpuUtilization;
	float socTempC;
} IOKitMetrics;

static IOReportSubscriptionRef g_subscription = NULL;
static CFMutableDictionaryRef g_channels = NULL;
static io_connect_t g_smcConn = 0;

static char g_cpu_keys[64][5];
static int g_cpu_key_count = 0;
static char g_gpu_keys[64][5];
static int g_gpu_key_count = 0;

static int cfStringMatch(CFStringRef str, const char *match) {
	if (str == NULL || match == NULL)
		return 0;
	CFStringRef matchStr = CFStringCreateWithCString(kCFAllocatorDefault, match,
		kCFStringEncodingUTF8);
	if (matchStr == NULL)
		return 0;
	int result = (CFStringCompare(str, matchStr, 0) == kCFCompareEqualTo);
	CFRelease(matchStr);
	return result;
}

static int cfStringContains(CFStringRef str, const char *substr) {
	if (str == NULL || substr == NULL)
		return 0;
	CFStringRef substrRef = CFStringCreateWithCString(kCFAllocatorDefault, substr,
		kCFStringEncodingUTF8);
	if (substrRef == NULL)
		return 0;
	CFRange result = CFStringFind(str, substrRef, 0);
	CFRelease(substrRef);
	return (result.location != kCFNotFound);
}

static int cfStringStartsWith(CFStringRef str, const char *prefix) {
	if (str == NULL || prefix == NULL)
		return 0;
	CFStringRef prefixRef = CFStringCreateWithCString(kCFAllocatorDefault, prefix,
		kCFStringEncodingUTF8);
	if (prefixRef == NULL)
		return 0;
	int result = CFStringHasPrefix(str, prefixRef);
	CFRelease(prefixRef);
	return result;
}

static double energyToWatts(int64_t energy, CFStringRef unitRef,
	double durationMs) {
	if (durationMs <= 0)
		durationMs = 1;
	double val = (double)energy;
	double rate = val / (durationMs / 1000.0);

	if (unitRef == NULL)
		return rate / 1e6;

	char unit[32] = {0};
	CFStringGetCString(unitRef, unit, sizeof(unit), kCFStringEncodingUTF8);

	for (int i = 0; unit[i]; i++) {
		if (unit[i] == ' ')
			unit[i] = '\0';
	}

	if (strcmp(unit, "mJ") == 0) {
		return rate / 1e3;
	} else if (strcmp(unit, "uJ") == 0) {
		return rate / 1e6;
	} else if (strcmp(unit, "nJ") == 0) {
		return rate / 1e9;
	}
	return rate / 1e6;
}

static void loadSMCTempKeys(void) {
	if (g_cpu_key_count > 0 || g_gpu_key_count > 0)
		return;

	if (!g_smcConn)
		return;

	int totalKeys = SMCGetKeyCount(g_smcConn);
	for (int i = 0; i < totalKeys; i++) {
		char key[5];
		if (SMCGetKeyFromIndex(g_smcConn, i, key) != kIOReturnSuccess)
			continue;

		SMCKeyData_keyInfo_t keyInfo;
		if (SMCGetKeyInfo(g_smcConn, key, &keyInfo) != kIOReturnSuccess)
			continue;

		if (keyInfo.dataType != 1718383648)
			continue;

		if ((key[0] == 'T' && (key[1] == 'p' || key[1] == 'e'))) {
			if (g_cpu_key_count < 64) {
				strcpy(g_cpu_keys[g_cpu_key_count++], key);
			}
		} else if (key[0] == 'T' && key[1] == 'g') {
			if (g_gpu_key_count < 64) {
				strcpy(g_gpu_keys[g_gpu_key_count++], key);
			}
		}
	}
}

static float readSMCTemperature(float *outCpuTemp, float *outGpuTemp) {
	float cpuSum = 0;
	int cpuCount = 0;
	float gpuSum = 0;
	int gpuCount = 0;

	if (g_smcConn) {
		for (int i = 0; i < g_cpu_key_count; i++) {
			float val = (float)SMCGetFloatValue(g_smcConn, g_cpu_keys[i]);
			if (val > 0) {
				cpuSum += val;
				cpuCount++;
			}
		}
		for (int i = 0; i < g_gpu_key_count; i++) {
			float val = (float)SMCGetFloatValue(g_smcConn, g_gpu_keys[i]);
			if (val > 0) {
				gpuSum += val;
				gpuCount++;
			}
		}
	}

	if (cpuCount > 0)
		*outCpuTemp = cpuSum / cpuCount;
	if (gpuCount > 0)
		*outGpuTemp = gpuSum / gpuCount;

	if (gpuCount > 0)
		return *outGpuTemp;
	if (cpuCount > 0)
		return *outCpuTemp;
	return 0.0f;
}

int initIOKit(void) {
	if (g_channels != NULL)
		return 0;

	CFStringRef energyGroup = CFSTR("Energy Model");
	CFStringRef gpuGroup = CFSTR("GPU Stats");
	CFStringRef cpuGroup = CFSTR("CPU Stats");

	CFDictionaryRef energyChan =
		IOReportCopyChannelsInGroup(energyGroup, NULL, 0, 0, 0);
	if (energyChan == NULL)
		return -1;

	CFDictionaryRef gpuChan = IOReportCopyChannelsInGroup(gpuGroup, NULL, 0, 0, 0);
	if (gpuChan != NULL) {
		IOReportMergeChannels(energyChan, gpuChan, NULL);
		CFRelease(gpuChan);
	}

	CFDictionaryRef cpuChan = IOReportCopyChannelsInGroup(cpuGroup, NULL, 0, 0, 0);
	if (cpuChan != NULL) {
		IOReportMergeChannels(energyChan, cpuChan, NULL);
		CFRelease(cpuChan);
	}

	CFIndex size = CFDictionaryGetCount(energyChan);
	g_channels = CFDictionaryCreateMutableCopy(kCFAllocatorDefault, size, energyChan);
	CFRelease(energyChan);

	if (g_channels == NULL)
		return -2;

	CFMutableDictionaryRef subsystem = NULL;
	g_subscription = IOReportCreateSubscription(NULL, g_channels, &subsystem, 0, NULL);

	if (g_subscription == NULL) {
		CFRelease(g_channels);
		g_channels = NULL;
		return -3;
	}

	g_smcConn = SMCOpen();
	loadSMCTempKeys();

	return 0;
}

IOKitMetrics sampleIOKitMetrics(int durationMs) {
	IOKitMetrics m = {0, 0, 0, 0, 0, 0};

	if (g_subscription == NULL || g_channels == NULL) {
		if (initIOKit() != 0)
			return m;
	}

	CFDictionaryRef sample1 = IOReportCreateSamples(g_subscription, g_channels, NULL);
	if (sample1 == NULL)
		return m;

	usleep((useconds_t)(durationMs * 1000));

	CFDictionaryRef sample2 = IOReportCreateSamples(g_subscription, g_channels, NULL);
	if (sample2 == NULL) {
		CFRelease(sample1);
		return m;
	}

	CFDictionaryRef delta = IOReportCreateSamplesDelta(sample1, sample2, NULL);
	CFRelease(sample1);
	CFRelease(sample2);

	if (delta == NULL)
		return m;

	CFArrayRef channels = CFDictionaryGetValue(delta, CFSTR("IOReportChannels"));
	if (channels == NULL) {
		CFRelease(delta);
		return m;
	}

	CFIndex count = CFArrayGetCount(channels);
	for (CFIndex i = 0; i < count; i++) {
		CFDictionaryRef item = (CFDictionaryRef)CFArrayGetValueAtIndex(channels, i);
		if (item == NULL)
			continue;

		CFStringRef groupRef = IOReportChannelGetGroup(item);
		CFStringRef channelRef = IOReportChannelGetChannelName(item);

		if (groupRef == NULL || channelRef == NULL)
			continue;

		if (cfStringMatch(groupRef, "Energy Model")) {
			CFStringRef unitRef = IOReportChannelGetUnitLabel(item);
			int64_t val = IOReportSimpleGetIntegerValue(item, 0);
			double watts = energyToWatts(val, unitRef, (double)durationMs);

			if (cfStringContains(channelRef, "CPU Energy")) {
				m.cpuPowerW += watts;
			} else if (cfStringMatch(channelRef, "GPU Energy")) {
				m.gpuPowerW += watts;
			} else if (cfStringStartsWith(channelRef, "ANE")) {
				m.anePowerW += watts;
			}
		} else if (cfStringMatch(groupRef, "GPU Stats")) {
			CFStringRef subgroupRef = IOReportChannelGetSubGroup(item);
			if (subgroupRef != NULL &&
				cfStringMatch(subgroupRef, "GPU Performance States")) {
				if (cfStringMatch(channelRef, "GPUPH")) {
					int32_t stateCount = IOReportStateGetCount(item);
					int64_t totalTime = 0;
					int64_t activeTime = 0;

					for (int32_t s = 0; s < stateCount; s++) {
						int64_t residency = IOReportStateGetResidency(item, s);
						CFStringRef stateName = IOReportStateGetNameForIndex(item, s);
						totalTime += residency;

						if (stateName != NULL && !cfStringMatch(stateName, "OFF") &&
							!cfStringMatch(stateName, "IDLE") &&
							!cfStringMatch(stateName, "DOWN")) {
							activeTime += residency;
						}
					}

					if (totalTime > 0) {
						m.gpuUtilization = (double)activeTime / (double)totalTime;
					}
				}
			}
		}
	}

	CFRelease(delta);

	float cpuTemp = 0, gpuTemp = 0;
	m.socTempC = readSMCTemperature(&cpuTemp, &gpuTemp);

	if (g_smcConn) {
		m.systemPowerW = SMCGetFloatValue(g_smcConn, "PSTR");
	}

	return m;
}

void cleanupIOKit(void) {
	if (g_channels != NULL) {
		CFRelease(g_channels);
		g_channels = NULL;
	}
	g_subscription = NULL;
	g_cpu_key_count = 0;
	g_gpu_key_count = 0;
	if (g_smcConn) {
		SMCClose(g_smcConn);
		g_smcConn = 0;
	}
}
