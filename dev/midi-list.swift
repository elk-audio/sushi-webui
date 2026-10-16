// Lists the CoreMIDI sources as "<index> <name>". The index is the RtMidi input device number that
// Sushi's rt_midi_input_mappings uses. Used by run-sushi.sh.
import CoreMIDI

for i in 0..<MIDIGetNumberOfSources() {
    var name: Unmanaged<CFString>?
    MIDIObjectGetStringProperty(MIDIGetSource(i), kMIDIPropertyDisplayName, &name)
    print(i, name?.takeRetainedValue() as String? ?? "?")
}
