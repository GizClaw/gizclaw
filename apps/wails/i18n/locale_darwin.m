#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

char *gizclawPreferredLocale(void) {
  @autoreleasepool {
    NSString *locale = [NSLocale preferredLanguages].firstObject;
    if (locale == nil) return NULL;
    return strdup(locale.UTF8String);
  }
}
