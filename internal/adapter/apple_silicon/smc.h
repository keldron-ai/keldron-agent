// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)
// SMC interface adapted from mactop (MIT).

#ifndef SMC_H
#define SMC_H

#ifdef __APPLE__
#include <IOKit/IOKitLib.h>
#else
#error "smc.h requires macOS (IOKit)"
#endif

#define KERNEL_INDEX_SMC 2

#define SMC_CMD_READ_BYTES 5
#define SMC_CMD_WRITE_BYTES 6
#define SMC_CMD_READ_INDEX 8
#define SMC_CMD_READ_KEYINFO 9
#define SMC_CMD_READ_PLIMIT 11
#define SMC_CMD_READ_VERS 12

// FourCC for 'flt ' data type: (('f' << 24) | ('l' << 16) | ('t' << 8) | ' ')
#define kSMCDataTypeFloat 1718383648

typedef struct {
	char major;
	char minor;
	char build;
	char reserved[1];
	unsigned short release;
} SMCKeyData_vers_t;

typedef struct {
	unsigned short version;
	unsigned short length;
	unsigned int cpuPLimit;
	unsigned int gpuPLimit;
	unsigned int memPLimit;
} SMCKeyData_pLimitData_t;

typedef struct {
	unsigned int dataSize;
	unsigned int dataType;
	char dataAttributes;
} SMCKeyData_keyInfo_t;

typedef char SMCBytes_t[32];

typedef struct {
	unsigned int key;
	SMCKeyData_vers_t vers;
	SMCKeyData_pLimitData_t pLimitData;
	SMCKeyData_keyInfo_t keyInfo;
	char result;
	char status;
	char data8;
	unsigned int data32;
	SMCBytes_t bytes;
} SMCKeyData_t;

typedef char SMCKey_t[5];

typedef struct {
	char key[4];
	SMCKeyData_t data;
} SMCVal_t;

io_connect_t SMCOpen(void);
kern_return_t SMCClose(io_connect_t conn);
kern_return_t SMCReadKey(io_connect_t conn, const char *key, SMCKeyData_t *val);
double SMCGetFloatValue(io_connect_t conn, const char *key);
int SMCGetKeyCount(io_connect_t conn);
kern_return_t SMCGetKeyFromIndex(io_connect_t conn, int index, char *outputKey);
kern_return_t SMCGetKeyInfo(io_connect_t conn, const char *key,
	SMCKeyData_keyInfo_t *keyInfo);

#endif
