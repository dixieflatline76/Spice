#define VerFileNum FileOpen("version.txt")
#define MyAppVersion Trim(StringChange(FileRead(VerFileNum),"v",""))

[Setup]
AppMutex=Spice_SingleInstanceMutex
AppName=Spice
AppVersion={#MyAppVersion}
ArchitecturesAllowed=x64compatible and not arm64
DefaultDirName={commonpf64}\Spice
DefaultGroupName=Spice
OutputBaseFilename=Spice-Setup
AppPublisher=Karl Kwong
SetupIconFile=Spice.ico
Compression=lzma2
SolidCompression=yes
OutputDir=bin
WizardStyle=modern
AppVerName=Spice - Wallpaper Manager v{#MyAppVersion}
DisableWelcomePage=no
WizardImageFile=Spice.bmp
UninstallDisplayIcon={app}\Spice.exe
DisableProgramGroupPage=Yes

[Files]
Source: bin\Spice.exe; DestDir: {app}; Flags: ignoreversion
Source: Spice.ico; DestDir: {app}; Flags: ignoreversion

[Icons]
Name: {group}\Spice; Filename: {app}\Spice.exe; IconFilename: {app}\Spice.ico

[Tasks]
Name: "startonboot"; Description: "Start with Windows"; GroupDescription: "Additional tasks"; Flags: unchecked

[Registry]
Root: HKCU; Subkey: "SOFTWARE\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "Spice"; ValueData: """{app}\Spice.exe"""; Flags: uninsdeletekey; Tasks: startonboot

[UninstallDelete]
Type: files; Name: "{userappdata}\fyne\Spice\preferences.json"
Type: filesandordirs; Name: "{localappdata}\Temp\spice_downloads"
Type: filesandordirs; Name: "{commonstartup}\Spice"

[Run]
Filename: {app}\Spice; Description: Start Spice; Flags: postinstall nowait skipifsilent unchecked

[Code]
procedure InitializeWizard;
var
  RichViewer: TRichEditViewer;
begin
  RichViewer := TRichEditViewer.Create(WizardForm);
  RichViewer.Left := WizardForm.WelcomeLabel2.Left;
  RichViewer.Top := WizardForm.WelcomeLabel2.Top;
  RichViewer.Width := WizardForm.WelcomeLabel2.Width;
  RichViewer.Height := WizardForm.WelcomeLabel2.Height;
  RichViewer.Parent := WizardForm.WelcomeLabel2.Parent;
  RichViewer.BorderStyle := bsNone;
  RichViewer.TabStop := False;
  RichViewer.ReadOnly := True;
  WizardForm.WelcomeLabel2.Visible := False;
   
  RichViewer.RTFText:=
    '{\rtf2\ansi\deff0 {\fonttbl {\f0 Arial;}} \
    {\colortbl\red0\green0\blue0;} \
    \par \
    \fs18 Spice is a wallpaper manager for Windows, \
    inspired by the popular Linux wallpaper manager, Variety.\par \
    \par \
    \fs18 You are installing version \b v{#MyAppVersion} \b0 \par \
    \par \
    \fs18 Be sure to check out the project page on GitHub for the latest news, updates, and to contribute:\par \
    {\field{\*\fldinst{HYPERLINK "https://github.com/dixieflatline76/Spice"}}{\\fldrslt https://github.com/dixieflatline76/Spice}} \
    }';
end;