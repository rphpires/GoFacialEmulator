/* ============================================================================
   W-Access (Invenzi) — INSERT de Controlador Local (LocalController)
   ----------------------------------------------------------------------------
   Tabela assumida: dbo.CfgHWLocalControllers   <-- AJUSTE se o nome for outro.

   Por que INSERT manual (sem SP):
     - Não há stored procedure de provisionamento exposta.
     - LocalControllerID É IDENTITY → o banco gera a PK. NÃO incluir a coluna
       no INSERT (IDENTITY_INSERT OFF). ID capturado via SCOPE_IDENTITY().

   Colunas NOT NULL (precisam de valor):
     LocalControllerID, LocalControllerName, LocalControllerDescription,
     PartitionID, LocalControllerType, IPAddress, BaseCommPort,
     LocalControllerEnabled, LocalControllerStatus,
     ForceLocalAuthentication, IsEnrollTerminal
   Colunas NULLable (opcionais — deixe NULL se não souber):
     SiteControllerID, CameraID, CameraPresetHWID, LocalityID,
     LocalControllerVersion, MacAddress, Platform, SerialNumber,
     ModelName, ExtraSettings (varchar(max))

   AVISO: schema do W-Access varia por versão. Valide os nomes/tipos das
   colunas contra o banco do cliente antes de rodar em produção.
   ============================================================================ */

SET NOCOUNT ON;
SET XACT_ABORT ON;   -- qualquer erro = rollback automático

/* ------------------------------------------------------------------ */
/* 1) PARÂMETROS — edite apenas este bloco                            */
/* ------------------------------------------------------------------ */
DECLARE
    /* --- obrigatórios --- */
    @LocalControllerName        varchar(50)   = N'Emulador Dahua',
    @LocalControllerDescription varchar(100)  = N'emulator_20',
    @PartitionID                int           = 0,        -- 0=Sistema (equipamentos/config). 1=Usuários. Controlador é equipamento → 0.
    @LocalControllerType        smallint      = 22121,        -- código do tipo de device
    @IPAddress                  varchar(100)  = N'dev-linux',
    @BaseCommPort               int           = 4001,
    @LocalControllerEnabled     bit           = 1,
    @LocalControllerStatus      int           = 0,        -- 0 = estado inicial/offline
    @ForceLocalAuthentication   bit           = 1,
    @IsEnrollTerminal           bit           = 0,

    /* --- opcionais (NULL = não preencher) --- */
    @SiteControllerID           int           = 2,
    @CameraID                   int           = NULL,
    @CameraPresetHWID           smallint      = NULL,
    @LocalityID                 int           = 3,
    @LocalControllerVersion     varchar(50)   = NULL,
    @MacAddress                 varchar(50)   = NULL,
    @Platform                   varchar(500)  = NULL,
    @SerialNumber               varchar(500)  = NULL,
    @ModelName                  varchar(500)  = NULL,
    @ExtraSettings              varchar(max)  = NULL;

/* ------------------------------------------------------------------ */
/* 2) INSERT idempotente + PK calculada                               */
/* ------------------------------------------------------------------ */
BEGIN TRAN;

    /* LocalControllerID é IDENTITY → o banco gera. Capturado via SCOPE_IDENTITY(). */
    DECLARE @NewID int;

    IF EXISTS (SELECT 1 FROM dbo.CfgHWLocalControllers
               WHERE LocalControllerName = @LocalControllerName)
    BEGIN
        PRINT N'>> Já existe controlador com nome "' + @LocalControllerName
            + N'". Nada inserido.';
    END
    ELSE
    BEGIN
        INSERT INTO dbo.CfgHWLocalControllers
        (
            LocalControllerName,
            LocalControllerDescription,
            SiteControllerID,
            PartitionID,
            LocalControllerType,
            IPAddress,
            BaseCommPort,
            LocalControllerEnabled,
            LocalControllerStatus,
            CameraID,
            CameraPresetHWID,
            LocalityID,
            LocalControllerVersion,
            MacAddress,
            ForceLocalAuthentication,
            IsEnrollTerminal,
            Platform,
            SerialNumber,
            ModelName,
            ExtraSettings
        )
        VALUES
        (
            @LocalControllerName,
            @LocalControllerDescription,
            @SiteControllerID,
            @PartitionID,
            @LocalControllerType,
            @IPAddress,
            @BaseCommPort,
            @LocalControllerEnabled,
            @LocalControllerStatus,
            @CameraID,
            @CameraPresetHWID,
            @LocalityID,
            @LocalControllerVersion,
            @MacAddress,
            @ForceLocalAuthentication,
            @IsEnrollTerminal,
            @Platform,
            @SerialNumber,
            @ModelName,
            @ExtraSettings
        );

        SET @NewID = CAST(SCOPE_IDENTITY() AS int);

        PRINT N'>> Controlador inserido. LocalControllerID = '
            + CAST(@NewID AS varchar(10));
    END

COMMIT TRAN;

/* ------------------------------------------------------------------ */
/* 3) Conferência                                                     */
/* ------------------------------------------------------------------ */
SELECT LocalControllerID, LocalControllerName, IPAddress, BaseCommPort,
       PartitionID, LocalControllerType, LocalControllerEnabled, LocalControllerStatus
FROM dbo.CfgHWLocalControllers
WHERE LocalControllerName = @LocalControllerName;
