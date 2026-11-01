// Renders an SVG into a square PNG with a real alpha channel.
// Usage: swift scripts/render-icon.swift in.svg out.png size
import AppKit

let args = CommandLine.arguments
guard args.count == 4, let size = Int(args[3]), size > 0 else {
	FileHandle.standardError.write(Data("usage: render-icon in.svg out.png size\n".utf8))
	exit(2)
}
guard let image = NSImage(contentsOfFile: args[1]) else {
	FileHandle.standardError.write(Data("render-icon: cannot load \(args[1]) as an image\n".utf8))
	exit(1)
}
guard let rep = NSBitmapImageRep(
	bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8,
	samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
	colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 32),
	let ctx = NSGraphicsContext(bitmapImageRep: rep) else {
	FileHandle.standardError.write(Data("render-icon: cannot create bitmap\n".utf8))
	exit(1)
}
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = ctx
NSColor.clear.set()
NSRect(x: 0, y: 0, width: size, height: size).fill(using: .copy)
image.draw(in: NSRect(x: 0, y: 0, width: size, height: size), from: .zero, operation: .sourceOver, fraction: 1.0)
NSGraphicsContext.restoreGraphicsState()
guard let png = rep.representation(using: .png, properties: [:]) else {
	FileHandle.standardError.write(Data("render-icon: PNG encoding failed\n".utf8))
	exit(1)
}
try png.write(to: URL(fileURLWithPath: args[2]))
