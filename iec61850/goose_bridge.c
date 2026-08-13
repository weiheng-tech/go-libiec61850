#include <stdint.h>
#include <stdlib.h>

#include "goose_publisher.h"
#include "goose_subscriber.h"
#include "linked_list.h"
#include "mms_value.h"
#include "_cgo_export.h"

#ifndef GOOSE_FLAT_MMS_VALUE_DEFINED
#define GOOSE_FLAT_MMS_VALUE_DEFINED
typedef struct {
    int typ;
    int64_t ival;
    double dval;
    const char* sval;
} GooseFlatMmsValue;
#endif

#define FLAT_STRUCT_START 100
#define FLAT_STRUCT_END 101
#define FLAT_ARRAY_START 102
#define FLAT_ARRAY_END 103

static int countMmsElements(MmsValue* value) {
    if (value == NULL)
        return 0;

    MmsType type = MmsValue_getType(value);
    if (type != MMS_STRUCTURE && type != MMS_ARRAY)
        return 1;

    int count = 2;
    int size = MmsValue_getArraySize(value);
    for (int i = 0; i < size; i++)
        count += countMmsElements(MmsValue_getElement(value, i));
    return count;
}

static void flattenMmsValue(MmsValue* value, GooseFlatMmsValue* values, int* index, int capacity) {
    if (value == NULL || *index >= capacity)
        return;

    MmsType type = MmsValue_getType(value);
    values[*index].typ = type;

    switch (type) {
    case MMS_BOOLEAN:
        values[*index].ival = MmsValue_getBoolean(value);
        (*index)++;
        break;
    case MMS_FLOAT:
        values[*index].dval = MmsValue_toDouble(value);
        (*index)++;
        break;
    case MMS_INTEGER:
    case MMS_UNSIGNED:
        values[*index].ival = MmsValue_toInt64(value);
        (*index)++;
        break;
    case MMS_STRING:
    case MMS_VISIBLE_STRING:
    case MMS_OCTET_STRING:
        values[*index].sval = MmsValue_toString(value);
        (*index)++;
        break;
    case MMS_BIT_STRING:
        values[*index].ival = MmsValue_getBitStringAsInteger(value);
        (*index)++;
        break;
    case MMS_UTC_TIME:
        values[*index].ival = MmsValue_toUnixTimestamp(value);
        (*index)++;
        break;
    case MMS_STRUCTURE:
    case MMS_ARRAY: {
        values[*index].typ = type == MMS_STRUCTURE ? FLAT_STRUCT_START : FLAT_ARRAY_START;
        (*index)++;
        int size = MmsValue_getArraySize(value);
        for (int i = 0; i < size; i++)
            flattenMmsValue(MmsValue_getElement(value, i), values, index, capacity);
        if (*index < capacity) {
            values[*index].typ = type == MMS_STRUCTURE ? FLAT_STRUCT_END : FLAT_ARRAY_END;
            (*index)++;
        }
        break;
    }
    default:
        (*index)++;
        break;
    }
}

GooseFlatMmsValue* flattenGooseDataSet(GooseSubscriber subscriber, int* outCount) {
    MmsValue* dataSet = GooseSubscriber_getDataSetValues(subscriber);
    if (dataSet == NULL) {
        *outCount = 0;
        return NULL;
    }

    int count = countMmsElements(dataSet);
    if (count == 0) {
        *outCount = 0;
        return NULL;
    }

    GooseFlatMmsValue* values = calloc((size_t)count, sizeof(GooseFlatMmsValue));
    if (values == NULL) {
        *outCount = 0;
        return NULL;
    }

    int index = 0;
    flattenMmsValue(dataSet, values, &index, count);
    *outCount = index;
    return values;
}

static void gooseListenerBridge(GooseSubscriber subscriber, void* parameter) {
    goGooseCallback((GoInt)(intptr_t)parameter, (void*)subscriber);
}

void installGooseListener(GooseSubscriber subscriber, int handlerID) {
    GooseSubscriber_setListener(subscriber, gooseListenerBridge, (void*)(intptr_t)handlerID);
}

void destroyGooseDataSet(LinkedList dataSet) {
    LinkedList_destroyDeep(dataSet, (LinkedListValueDeleteFunction)MmsValue_delete);
}

CommParameters* createGooseCommParameters(uint8_t vlanPriority, uint16_t vlanId,
                                          uint16_t appId, const uint8_t* destination) {
    CommParameters* parameters = calloc(1, sizeof(CommParameters));
    if (parameters == NULL)
        return NULL;
    parameters->vlanPriority = vlanPriority;
    parameters->vlanId = vlanId;
    parameters->appId = appId;
    for (int i = 0; i < 6; i++)
        parameters->dstAddress[i] = destination[i];
    return parameters;
}
