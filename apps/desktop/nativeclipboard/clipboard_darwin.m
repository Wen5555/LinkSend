#import <Cocoa/Cocoa.h>
#import <ImageIO/ImageIO.h>
#include <stdlib.h>
#include <string.h>

static const size_t LinkSendMaxImageBytes = 32 * 1024 * 1024;
static const uint64_t LinkSendMaxImagePixels = 40000000;
static const uint64_t LinkSendMaxImageDimension = 32768;

typedef struct {
    NSMutableData *data;
    bool overflow;
} LinkSendPNGBuffer;

static size_t linksendPNGWrite(void *info, const void *buffer, size_t count) {
    LinkSendPNGBuffer *output = (LinkSendPNGBuffer *)info;
    if (count > LinkSendMaxImageBytes - output->data.length) {
        output->overflow = true;
        return 0;
    }
    [output->data appendBytes:buffer length:count];
    return count;
}

// Only losslessly snapshots pixels. No file URL dereference, HTML rendering,
// implicit URL launch, clipboard mutation or JavaScript bridge is involved.
int linksendClipboardPNG(const char *name, void **output, size_t *size) {
    *output = NULL;
    *size = 0;
    @autoreleasepool {
        @try {
            NSPasteboard *pasteboard = name ? [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:name]] : NSPasteboard.generalPasteboard;
            if (!pasteboard) return 4;
            NSInteger changeCount = pasteboard.changeCount;
            NSPasteboardType type = [pasteboard availableTypeFromArray:@[NSPasteboardTypePNG, NSPasteboardTypeTIFF]];
            if (!type) return 1;
            NSData *data = [pasteboard dataForType:type];
            if (pasteboard.changeCount != changeCount) return 3;
            if (!data) return 4;
            if (data.length == 0 || data.length > LinkSendMaxImageBytes) return 2;
            NSDictionary *options = @{(NSString *)kCGImageSourceShouldCache: @NO};
            CGImageSourceRef source = CGImageSourceCreateWithData((CFDataRef)data, (CFDictionaryRef)options);
            if (!source || CGImageSourceGetCount(source) != 1) { if (source) CFRelease(source); return 4; }
            CFDictionaryRef rawProperties = CGImageSourceCopyPropertiesAtIndex(source, 0, NULL);
            NSDictionary *properties = (NSDictionary *)rawProperties;
            uint64_t width = [properties[(NSString *)kCGImagePropertyPixelWidth] unsignedLongLongValue];
            uint64_t height = [properties[(NSString *)kCGImagePropertyPixelHeight] unsignedLongLongValue];
            if (rawProperties) CFRelease(rawProperties);
            if (width == 0 || height == 0 || width > LinkSendMaxImageDimension || height > LinkSendMaxImageDimension || width > LinkSendMaxImagePixels / height) { CFRelease(source); return 2; }
            CGImageRef image = CGImageSourceCreateImageAtIndex(source, 0, (CFDictionaryRef)options);
            CFRelease(source);
            if (!image) return 4;
            LinkSendPNGBuffer buffer = {[NSMutableData data], false};
            CGDataConsumerCallbacks callbacks = {linksendPNGWrite, NULL};
            CGDataConsumerRef consumer = CGDataConsumerCreate(&buffer, &callbacks);
            if (!consumer) { CGImageRelease(image); return 4; }
            CGImageDestinationRef destination = CGImageDestinationCreateWithDataConsumer(consumer, CFSTR("public.png"), 1, NULL);
            CGDataConsumerRelease(consumer);
            if (!destination) { CGImageRelease(image); return 4; }
            CGImageDestinationAddImage(destination, image, NULL);
            CGImageRelease(image);
            bool ok = CGImageDestinationFinalize(destination);
            CFRelease(destination);
            if (buffer.overflow) return 2;
            if (!ok || buffer.data.length == 0) return 4;
            if (pasteboard.changeCount != changeCount) return 3;
            *output = malloc(buffer.data.length);
            if (!*output) return 4;
            *size = buffer.data.length;
            memcpy(*output, buffer.data.bytes, *size);
            return 0;
        } @catch (NSException *exception) {
            (void)exception;
            return 4;
        }
    }
}
